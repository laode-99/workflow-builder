package leadflow

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/workflow-builder/core/internal/integrations/leadsquared"
	"github.com/workflow-builder/core/internal/model"
	"github.com/workflow-builder/core/internal/repo"
	"github.com/workflow-builder/core/internal/sdk"
)

// RunCRMSyncPoller is the singleton background job that processes pending CRM intents.
func RunCRMSyncPoller(ctx context.Context, exec sdk.Execution) error {
	log := exec.Logger()
	db := exec.GetDB()
	intentRepo := repo.NewCRMIntentRepo(db)

	// 1. Fetch a batch of pending intents
	intents, err := intentRepo.GetPendingForProcessing(ctx, 20)
	if err != nil {
		return fmt.Errorf("fetch pending intents: %v", err)
	}

	if len(intents) == 0 {
		return nil
	}

	log.Infof("Processing %d CRM sync intents...", len(intents))

	for _, intent := range intents {
		// Process each intent
		err := processIntent(ctx, exec, intent)
		if err != nil {
			log.Errorf("CRM Sync failed for intent %s: %v", intent.ID, err)
			intentRepo.MarkFailed(ctx, intent.ID, err.Error())
		} else {
			log.Infof("CRM Sync success for lead %s (intent %s)", intent.LeadID, intent.ID)
			intentRepo.MarkDone(ctx, intent.ID)
		}
	}

	return nil
}

func processIntent(ctx context.Context, exec sdk.Execution, intent model.CRMSyncIntent) error {
	db := exec.GetDB()
	
	// 1. Load Lead
	var lead model.Lead
	if err := db.Preload("Business").First(&lead, "id = ?", intent.LeadID).Error; err != nil {
		return fmt.Errorf("lead not found: %v", err)
	}

	// 2. Load Credentials
	lsCredsRaw, err := exec.GetCredential("leadsquared")
	if err != nil {
		return fmt.Errorf("missing leadsquared credentials: %v", err)
	}
	var lsCreds leadsquared.Credentials
	_ = json.Unmarshal([]byte(lsCredsRaw), &lsCreds)
	
	lsClient := leadsquared.NewClient(lsCreds)

	// 3. Resolve Sales Assignment (if any)
	var sales *model.SalesAssignment
	var assignment model.LeadSalesAssignment
	if err := db.Where("lead_id = ?", lead.ID).First(&assignment).Error; err == nil {
		db.First(&sales, "id = ?", assignment.SalesAssignmentID)
	}

	// 4. Map fields based on the Path label on the intent (A or B).
	// Path A = responded/interest detected; Path B = no response/invalid.
	// The sales assignment is used only for the developer dispatch step,
	// not for the CRM field mapping itself.
	_ = sales
	updates := leadsquared.BuildUpdatesForPath(&lead, intent.Path)

	// 5. Execute API call
	err = lsClient.UpdateOpportunity(ctx, lead.ExternalID, updates)
	if err != nil {
		return fmt.Errorf("leadsquared api: %v", err)
	}

	return nil
}
