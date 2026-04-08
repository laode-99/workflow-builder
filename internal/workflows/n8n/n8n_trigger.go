package n8n

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/workflow-builder/core/internal/integrations/webhook"
	"github.com/workflow-builder/core/internal/sdk"
)

func init() {
	sdk.RegisterWorkflow("N8NTriggerWorkflow", sdk.WorkflowDef{
		Name:        "start flow in n8n",
		Description: "Trigger an external n8n workflow via a Webhook node. Supports custom JSON payloads.",
		Params: []sdk.Param{
			{
				Key:         "webhook_url",
				Type:        sdk.ParamTypeString,
				Description: "The 'Production URL' from your n8n Webhook node.",
			},
			{
				Key:         "json_payload",
				Type:        sdk.ParamTypeString,
				Description: "Optional: JSON data to send (e.g. {\"lead_id\": \"123\"}).",
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

	if payload == "" {
		payload = "{}" // Default empty JSON object
	}

	client := webhook.NewClient()
	
	log.Infof("🚀 Triggering n8n flow at: %s", webhookURL)
	
	resp, err := client.Trigger(ctx, webhookURL, []byte(payload))
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
			break
		}
		// Sleep periodically to avoid tight looping
		time.Sleep(3 * time.Second)
	}

	return nil
}
