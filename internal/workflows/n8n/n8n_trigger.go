package n8n

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/workflow-builder/core/internal/integrations/webhook"
	"github.com/workflow-builder/core/internal/sdk"
)

func init() {
	sdk.RegisterWorkflow("N8NTriggerWorkflow", sdk.WorkflowDef{
		Name:        "start flow in n8n",
		Description: "Trigger an external n8n workflow via a Webhook node. Supports custom JSON payloads. NOTE: This workflow is restricted and can only be executed between 08:00 AM and 06:00 PM (Jakarta time).",
		Params: []sdk.Param{
			{
				Key:         "webhook_url",
				Type:        sdk.ParamTypeString,
				Description: "The 'Production URL' from your n8n Webhook node.",
			},
			{
				Key:         "stop_endpoint_method",
				Type:        sdk.ParamTypeString,
				Description: "Optional: HTTP Method (e.g., GET, POST, DELETE) used to send the stop signal.",
				Optional:    true,
			},
			{
				Key:         "stop_endpoint_url",
				Type:        sdk.ParamTypeString,
				Description: "Optional: The HTTP URL to trigger when you click 'Stop' in the dashboard.",
				Optional:    true,
			},
			{
				Key:         "spreadsheet_link",
				Type:        sdk.ParamTypeString,
				Description: "Note: Place the URL to the Google Sheet used as the data source here.",
				Optional:    true,
			},
			{
				Key:         "json_payload",
				Type:        sdk.ParamTypeString,
				Description: "Optional: JSON data to send (e.g. {\"lead_id\": \"123\"}).",
				Optional:    true,
			},
			{
				Key:         "ignore_working_hours",
				Type:        sdk.ParamTypeString,
				Description: "Testing only: Type 'yes' to bypass the 8 AM - 6 PM safety restriction.",
				Optional:    true,
			},
		},
		Steps: []sdk.Step{
			{
				ID:          "prepare",
				Label:       "Encode",
				Icon:        "FileCode",
				Description: "Prepares the request payload for transmission.",
			},
			{
				ID:          "trigger",
				Label:       "n8n Trigger",
				Icon:        "Zap",
				Description: "Sends the POST request to n8n and waits for a success response.",
			},
		},
		Handler: N8NTriggerHandler,
	})
}

func N8NTriggerHandler(ctx context.Context, exec sdk.Execution) error {
	log := exec.Logger()
	webhookURL := exec.GetVar("webhook_url")
	payload := exec.GetVar("json_payload")

	if webhookURL == "" {
		return fmt.Errorf("webhook_url is required")
	}

	importURL, err := url.Parse(webhookURL)
	if err == nil {
		q := importURL.Query()
		q.Set("core_execution_id", exec.ID().String())
		importURL.RawQuery = q.Encode()
		webhookURL = importURL.String()
	}

	// Working hours validation: only run between 08:00 and 18:00 (Jakarta time)
	ignoreHours := exec.GetVar("ignore_working_hours")
	if ignoreHours != "yes" && ignoreHours != "true" {
		loc, err := time.LoadLocation("Asia/Jakarta")
		if err == nil {
			now := time.Now().In(loc)
			if now.Hour() < 8 || now.Hour() >= 18 {
				log.Errorf("🚫 Working hours restriction: This workflow can only run between 08:00 and 18:00. Current time: %s", now.Format("15:04"))
				return fmt.Errorf("workflow execution aborted: outside of operating hours (8 AM - 6 PM)")
			}
		}
	} else {
		log.Infof("⚠️ Working hours restriction BYPASSED for testing.")
	}

	if payload == "" {
		payload = "{}" // Default empty JSON object
	}

	// ALWAYS inject status: active and execution ID automatically into the JSON body
	var activeData map[string]interface{}
	if err := json.Unmarshal([]byte(payload), &activeData); err != nil {
		activeData = make(map[string]interface{})
	}
	activeData["status"] = "active"
	activeData["core_execution_id"] = exec.ID().String()
	finalPayload, _ := json.Marshal(activeData)

	client := webhook.NewClient()
	
	log.Infof("🚀 Triggering n8n flow at: %s", webhookURL)
	
	resp, err := client.Trigger(ctx, webhookURL, finalPayload)
	if err != nil {
		log.Errorf("✗ n8n trigger failed: %v", err)
		log.Errorf("Response body: %s", resp)
		return err
	}

	log.Infof("✓ n8n triggered successfully! Response: %s", resp)
	
	log.Infof("⏳ Workflow is now running externally in n8n. Waiting for manual stop...")
	for {
		if exec.ShouldStop() {
			log.Info("📍 Stop signal received! Terminating monitoring...")

			// Fire HTTP Stop Request if configured
			stopURL := exec.GetVar("stop_endpoint_url")
			if stopURL != "" {
				stopMethod := strings.ToUpper(strings.TrimSpace(exec.GetVar("stop_endpoint_method")))
				validMethods := map[string]bool{"GET": true, "POST": true, "PUT": true, "DELETE": true, "PATCH": true}
				
				if stopMethod == "" || !validMethods[stopMethod] {
					if stopMethod != "" {
						log.Warnf("⚠️ Invalid HTTP method '%s' provided. Defaulting to POST.", stopMethod)
					}
					stopMethod = "POST"
				}
				
				log.Infof("🛑 Sending explicit abort signal via HTTP request (%s %s)", stopMethod, stopURL)
				
				// Construct JSON payload with status so n8n can identify the stop signal
				stopPayload := []byte(fmt.Sprintf(`{"status":"stopped", "core_execution_id":"%s"}`, exec.ID().String()))
				
				// Standard HTTP client
				req, err := http.NewRequestWithContext(ctx, stopMethod, stopURL, bytes.NewBuffer(stopPayload))
				if err != nil {
					log.Errorf("✗ Failed to construct HTTP stop request: %v", err)
				} else {
					req.Header.Set("Content-Type", "application/json")
					httpClient := &http.Client{Timeout: 10 * time.Second}
					resp, err := httpClient.Do(req)
					if err != nil {
						log.Errorf("✗ Failed to send stop signal: %v", err)
					} else {
						defer resp.Body.Close()
						log.Infof("✓ explicit abort signal delivered! HTTP Status: %d", resp.StatusCode)
					}
				}
			}
			break
		}
		// Sleep periodically to avoid tight looping
		time.Sleep(3 * time.Second)
	}

	return nil
}
