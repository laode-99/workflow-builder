package leadflow

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/workflow-builder/core/internal/model"
	"github.com/workflow-builder/core/internal/repo"
	"github.com/workflow-builder/core/internal/sdk"
	"github.com/workflow-builder/core/internal/statemachine"
	"gorm.io/gorm"
)

func handleAttemptManager(ctx context.Context, exec sdk.Execution) error {
	log := exec.Logger()
	bizID := exec.BusinessID()
	db := exec.GetDB()
	leadRepo := repo.NewLeadRepo(db)

	log.Infof("AttemptManager running for project: %s", bizID)

	// 1. Fetch the project config
	var business model.Business
	if err := db.WithContext(ctx).First(&business, "id = ?", bizID).Error; err != nil {
		return fmt.Errorf("failed to load business config: %w", err)
	}

	if business.Status != "active" {
		log.Infof("Project %s is not active (status: %s), skipping attempt manager", bizID, business.Status)
		return nil
	}

	var flowCfg statemachine.FlowConfig
	if err := json.Unmarshal([]byte(business.Config), &flowCfg); err != nil {
		return fmt.Errorf("failed to parse FlowConfig: %w", err)
	}

	// Determine if we are within business hours right now.
	loc, err := time.LoadLocation(flowCfg.BusinessHours.Timezone)
	if err != nil {
		loc = time.UTC // fallback
	}
	now := time.Now().In(loc)
	inHours := statemachine.IsWithinBusinessHours(now, flowCfg.BusinessHours)

	// In the real environment, limit batch size per cron tick
	const batchSize = 100

	// Per-lead pending commands, collected during the transaction and enqueued
	// AFTER commit (the enqueue-after-commit pattern).
	type pendingEntry struct {
		businessID uuid.UUID
		leadID     uuid.UUID
		version    int
		commands   []statemachine.Command
	}
	var pending []pendingEntry

	err = db.Transaction(func(tx *gorm.DB) error {
		leads, err := leadRepo.GetDueForDispatch(ctx, tx, bizID, batchSize)
		if err != nil {
			return err
		}

		if len(leads) > 0 {
			log.Infof("Evaluating %d leads due for dispatch", len(leads))
		}

		for _, lead := range leads {
			patch, commands, err := statemachine.Transition(
				statemachine.LeadToStatemachine(lead),
				statemachine.EventCronTick{},
				flowCfg,
				now,
				inHours,
			)
			if err != nil {
				log.Errorf("State machine error for lead %s: %v", lead.ID, err)
				continue
			}
			if patch.IsEmpty() {
				continue
			}

			audit := repo.AuditEntry{
				Actor:     "attempt_manager",
				EventType: "attempt_advanced",
				Changes:   statemachine.PatchToMap(patch),
			}
			updated, err := leadRepo.TransitionTx(ctx, tx, lead.ID, lead.Version, patch, commands, audit)
			if err != nil {
				log.Warnf("Skipping lead %s due to version conflict: %v", lead.ID, err)
				continue
			}
			log.Infof("Lead %s transition applied (attempt %d→%d). Scheduled %d side effects.",
				lead.ID, lead.Attempt, updated.Attempt, len(commands))

			pending = append(pending, pendingEntry{
				businessID: bizID,
				leadID:     lead.ID,
				version:    updated.Version,
				commands:   commands,
			})
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("attempt manager transaction failed: %w", err)
	}

	// Enqueue-after-commit: each pending entry's commands become Asynq tasks
	// or CRM sync intents.
	for _, p := range pending {
		EnqueueCommands(ctx, p.businessID, p.leadID, p.version, p.commands)
	}
	return nil
}


