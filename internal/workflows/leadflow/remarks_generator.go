package leadflow

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/workflow-builder/core/internal/integrations/openai"
	"github.com/workflow-builder/core/internal/model"
	"github.com/workflow-builder/core/internal/repo"
	"github.com/workflow-builder/core/internal/sdk"
	"github.com/workflow-builder/core/internal/statemachine"
	"gorm.io/gorm"
)

func handleRemarks(ctx context.Context, exec sdk.Execution) error {
	log := exec.Logger()
	bizID := exec.BusinessID()
	db := exec.GetDB()

	cStateRepo := repo.NewChatbotStateRepo(db)
	msgRepo := repo.NewMessageRepo(db)
	promptRepo := repo.NewPromptRepo(db)
	leadRepo := repo.NewLeadRepo(db)
	_ = leadRepo // avoid unused

	log.Infof("RemarksGenerator running for project: %s", bizID)

	var business model.Business
	if err := db.WithContext(ctx).First(&business, "id = ?", bizID).Error; err != nil {
		return fmt.Errorf("load business: %w", err)
	}

	if business.Status != "active" {
		return nil
	}

	var flowCfg statemachine.FlowConfig
	if err := json.Unmarshal([]byte(business.Config), &flowCfg); err != nil {
		return fmt.Errorf("parse FlowConfig: %w", err)
	}

	delay := time.Duration(flowCfg.RemarksDelayHours) * time.Hour
	if delay == 0 {
		delay = 5 * time.Hour // default
	}

	// 1. List pending remarks
	states, err := cStateRepo.ListPendingRemarks(ctx, bizID, delay, 50)
	if err != nil {
		return err
	}

	if len(states) == 0 {
		return nil
	}

	log.Infof("Found %d leads pending remarks summary", len(states))

	// Get OpenAI client
	oaKey, err := exec.GetCredential("openai")
	if err != nil {
		return err
	}
	oaClient := openai.NewClient(oaKey)

	// Get Prompt
	promptRecord, err := promptRepo.GetActive(ctx, bizID, "remarks_generator")
	if err != nil {
		log.Warnf("No active remarks_generator prompt found for %s, skipping batch", bizID)
		return nil
	}

	for _, state := range states {
		// 2. Get history
		msgs, err := msgRepo.ListWindow(ctx, state.LeadID, 10) // last 10 turns
		if err != nil {
			log.Errorf("Failed to list messages for lead %s: %v", state.LeadID, err)
			continue
		}

		if len(msgs) == 0 {
			// Nothing to summarize? Mark as reset anyway to avoid infinite loop
			cStateRepo.WriteRemarks(ctx, state.LeadID, "")
			continue
		}

		historyStr := formatChatHistory(msgs)
		
		// 3. Call OpenAI
		resp, err := oaClient.ChatCompletion(ctx, openai.ChatRequest{
			Model: "gpt-4o-mini", // fallback or from config
			Messages: []openai.ChatMessage{
				{Role: "system", Content: promptRecord.Content},
				{Role: "user", Content: fmt.Sprintf("Conversation history:\n%s\n\nPlease provide the remarks summary.", historyStr)},
			},
			Temperature: 0.3,
		})
		if err != nil {
			log.Errorf("OpenAI remarks failed for lead %s: %v", state.LeadID, err)
			continue
		}

		remarks, _ := openai.ExtractText(resp)
		remarks = strings.TrimSpace(remarks)

		// 4. Update Lead & ChatbotState
		err = db.Transaction(func(tx *gorm.DB) error {
			// Update ChatbotState remarks
			if err := repo.NewChatbotStateRepo(tx).WriteRemarks(ctx, state.LeadID, remarks); err != nil {
				return err
			}

			// Prepend to Lead summary
			lead, err := repo.NewLeadRepo(tx).GetByID(ctx, state.LeadID)
			if err != nil {
				return err
			}

			newSummary := remarks
			if lead.Summary != "" {
				// Newest on top
				newSummary = remarks + "\n\n" + lead.Summary
			}

			patch := statemachine.Patch{
				Summary: &newSummary,
			}
			audit := repo.AuditEntry{
				Actor:     "remarks_generator",
				EventType: "remarks_generated",
				Reason:    "idle > 5h",
			}

			_, err = repo.NewLeadRepo(tx).TransitionTx(ctx, tx, state.LeadID, lead.Version, patch, audit)
			if err != nil {
				return err
			}

			// Enqueue CRM Sync
			// Note: We don't have a direct "EnqueueCmd" here yet that works outside Transition.
			// However, handleRemarks is a cron workflow, we can manually create a CRMSyncIntent.
			syncRepo := repo.NewCRMIntentRepo(tx)
			return syncRepo.CreateTx(ctx, tx, &model.CRMSyncIntent{
				BusinessID: bizID,
				LeadID:     state.LeadID,
				Path:       "A", // Remarks always follow interaction
				Payload:    "{}", // Worker will build full payload from lead state
				Status:     "pending",
			})
		})

		if err != nil {
			log.Errorf("Failed to save remarks for lead %s: %v", state.LeadID, err)
		} else {
			log.Infof("Remarks generated successfully for lead %s", state.LeadID)
		}
	}

	return nil
}

func formatChatHistory(msgs []model.LeadMessage) string {
	var sb strings.Builder
	for _, m := range msgs {
		role := "Customer"
		if m.Role == "assistant" {
			role = "AI Agent"
		}
		sb.WriteString(fmt.Sprintf("%s: %s\n", role, m.Content))
	}
	return sb.String()
}
