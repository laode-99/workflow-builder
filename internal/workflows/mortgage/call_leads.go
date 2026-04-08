package mortgage

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/workflow-builder/core/internal/integrations/gsheets"
	"github.com/workflow-builder/core/internal/integrations/retell"
	"github.com/workflow-builder/core/internal/sdk"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

func init() {
	sdk.RegisterWorkflow("MortgageCallWorkflow", sdk.WorkflowDef{
		Name:        "Mortgage Call",
		Description: "Reads leads from Google Sheets, filters uncalled rows, makes AI calls via Retell, and updates results via webhook.",
		Params: []sdk.Param{
			{Key: "google_credential_id", Type: sdk.ParamTypeCredential, Integration: "google_sheets", Description: "Select your Google Sheets API key"},
			{Key: "retell_credential_id", Type: sdk.ParamTypeCredential, Integration: "retell_ai", Description: "Select your Retell API key"},
			{Key: "google_sheet_id", Type: sdk.ParamTypeString, Description: "ID of the spreadsheet to read"},
			{Key: "google_sheet_tab_name", Type: sdk.ParamTypeString, Description: "e.g. Sheet1"},
			{Key: "retell_agent_id", Type: sdk.ParamTypeString, Description: "Retell AI Agent ID"},
			{Key: "from_number", Type: sdk.ParamTypeString, Description: "The phone number to call from"},
			{Key: "delay_seconds", Type: sdk.ParamTypeString, Description: "Wait time between calls (default: 7)"},
		},
		Steps: []sdk.Step{
			{ID: "fetch", Label: "Fetch Leads", Icon: "Database", Description: "Connects to Google Sheets and reads the specified tab content."},
			{ID: "filter", Label: "Filter", Icon: "Activity", Description: "Skips rows that already have a call summary or date."},
			{ID: "call", Label: "AI Call", Icon: "Phone", Description: "Initiates a Retell AI call with dynamic lead variables."},
			{ID: "webhook", Label: "Sync", Icon: "RefreshCw", Description: "Waits for callback to update the sheet with the call outcome."},
		},
		Handler: MortgageCallTrigger,
	})

	sdk.RegisterWebhook("RetellCallback", sdk.WebhookDef{
		Path: "/callbacks/retell",
		Func: MortgageCallWebhook,
	})
}

// MortgageCallTrigger — Main workflow: read sheet → filter → loop → call Retell
func MortgageCallTrigger(ctx context.Context, exec sdk.Execution) error {
	log := exec.Logger()

	sheetID := exec.GetVar("google_sheet_id")
	tabName := exec.GetVar("google_sheet_tab_name")
	agentID := exec.GetVar("retell_agent_id")
	fromNum := exec.GetVar("from_number")
	delay := exec.GetIntVar("delay_seconds", 7)

	// Get credentials — either from specific dropdown selection OR default fallback
	retellKey, err := exec.GetCredentialByVar("retell_credential_id")
	if err != nil {
		// Fallback to legacy behavior if not selected
		retellKey, err = exec.GetCredential("retell_ai")
		if err != nil {
			return err
		}
	}

	sheetsCred, err := exec.GetCredentialByVar("google_credential_id")
	if err != nil {
		// Fallback to legacy behavior if not selected
		sheetsCred, err = exec.GetCredential("google_sheets")
		if err != nil {
			return err
		}
	}

	// Build real clients
	retellAPI := retell.NewClient(retellKey)
	sheetsAPI, err := gsheets.NewClient(ctx, sheetsCred)
	if err != nil {
		return err
	}

	log.Infof("Starting Mortgage Call workflow for Sheet: %s / %s", sheetID, tabName)

	rows, err := sheetsAPI.ReadRows(ctx, sheetID, tabName+"!A:Z")
	if err != nil {
		return err
	}

	log.Infof("Fetched %d rows from sheet", len(rows))

	called := 0
	for _, row := range rows {
		if exec.IsStopped() {
			log.Info("Execution stopped by user.")
			return nil
		}

		// Filter: skip rows already called
		if row.Get("Summary") != "" || row.Get("Call_Date") != "" {
			continue
		}

		phoneNumber := row.Get("Phone Number")
		callDate := row.Get("Call Date")

		// Skip if already called
		if callDate != "" {
			log.Infof("⏭️ Skipping %s (already has Call Date: %s)", phoneNumber, callDate)
			continue
		}

		if phoneNumber == "" {
			continue
		}

		// Delay between calls to avoid rate limiting
		if called > 0 {
			log.Infof("⏳ Sequential gap: Waiting %d seconds before next call...", delay)
			time.Sleep(time.Duration(delay) * time.Second)
		}

		if exec.ShouldStop() {
			log.Infof("📍 Stop criteria reached. Cleaning up...")
			break
		}

		dynamicVars := map[string]interface{}{
			"first_name": row.Get("Name"),
		}

		metadata := map[string]string{
			"execution_id": exec.ID().String(),
		}

		log.Infof("Calling %s...", phoneNumber)
		
		// Ensure from number has +
		cleanFrom := fromNum
		if !strings.HasPrefix(cleanFrom, "+") {
			cleanFrom = "+" + cleanFrom
		}

		call, err := retellAPI.CreateCall(ctx, agentID, "+"+phoneNumber, cleanFrom, dynamicVars, metadata)
		if err != nil {
			log.Errorf("Call to %s failed: %v", phoneNumber, err)
			continue
		}
		log.Infof("Call created: %s", call.ID)
		called++
	}

	log.Infof("Batch complete. %d successful calls made.", called)
	
	// If we intended to make calls but none succeeded, mark as error
	if len(rows) > 0 && called == 0 {
		return fmt.Errorf("all attempted calls failed - check your credits and 'from_number' configuration")
	}

	return nil
}

// MortgageCallWebhook — Retell sends POST when call ends → update Google Sheet
func MortgageCallWebhook(ctx context.Context, db *gorm.DB, rdb *redis.Client, encKey []byte, query map[string]string, payload []byte) error {
	var webhook retell.WebhookPayload
	if err := json.Unmarshal(payload, &webhook); err != nil {
		return err
	}

	if webhook.Event != "call_ended" {
		return nil
	}

	executionIDStr := webhook.Call.Metadata["execution_id"]
	if executionIDStr == "" {
		return nil
	}

	// Reconstruct the execution context to get variables and credentials
	exec, err := sdk.ReconstructExecution(ctx, executionIDStr, db, rdb, encKey)
	if err != nil {
		return err
	}

	log := exec.Logger()
	log.Infof("Webhook received for call %s. Updating Sheet...", webhook.Call.ID)

	sheetID := exec.GetVar("google_sheet_id")
	tabName := exec.GetVar("google_sheet_tab_name")
	sheetsCred, err := exec.GetCredentialByVar("google_credential_id")
	if err != nil {
		sheetsCred, err = exec.GetCredential("google_sheets")
		if err != nil {
			return err
		}
	}

	sheetsAPI, err := gsheets.NewClient(ctx, sheetsCred)
	if err != nil {
		return err
	}

	cleanedNumber := strings.TrimPrefix(webhook.Call.ToNumber, "+")
	
	// Map Retell analysis to Sheet columns
	updates := map[string]interface{}{
		"Disconnected_Reason": webhook.Call.DisconnectionReason,
		"Interest":            webhook.Call.Analysis.CustomAnalysisData["Warm or Hot Leads"],
		"Summary":             webhook.Call.Analysis.CallSummary,
		"Call_Date":           time.Now().Format("2006-01-02 15:04:05"),
	}

	err = sheetsAPI.UpdateRowByMatch(ctx, sheetID, tabName+"!A:Z", "Phone Number", cleanedNumber, updates)
	if err != nil {
		log.Errorf("Failed to update sheet for %s: %v", cleanedNumber, err)
		return err
	}

	log.Infof("Successfully updated sheet for lead %s", cleanedNumber)
	return nil
}
