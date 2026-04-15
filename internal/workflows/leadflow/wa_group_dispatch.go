package leadflow

import (
	"context"
	"fmt"
	"time"

	"github.com/workflow-builder/core/internal/repo"
	"github.com/workflow-builder/core/internal/sdk"
	"github.com/workflow-builder/core/internal/statemachine"
	"gorm.io/gorm"
)

func handleWAGroup(ctx context.Context, exec sdk.Execution) error {
	log := exec.Logger()
	bizID := exec.BusinessID()
	db := exec.GetDB()

	leadRepo := repo.NewLeadRepo(db)
	salesRepo := repo.NewSalesRepo(db)

	log.Infof("WAGroupDispatcher running for project: %s", bizID)

	// 1. Fetch leads pending dispatch (Interest2 = Callback or Agent)
	leads, err := leadRepo.GetPendingWADispatch(ctx, bizID, 20)
	if err != nil {
		return err
	}

	if len(leads) == 0 {
		return nil
	}

	log.Infof("Found %d leads to dispatch to sales team", len(leads))

	for _, lead := range leads {
		// 2. Round-robin: select next active salesperson
		sales, err := salesRepo.GetNextForAssignment(ctx, bizID)
		if err != nil {
			if err == repo.ErrNotFound {
				log.Warnf("No active sales assignments found for project %s. Skipping dispatch.", bizID)
				return nil
			}
			log.Errorf("Failed to get sales assignment: %v", err)
			continue
		}

		log.Infof("Assigning lead %s to %s (WA Group: %s)", lead.ID, sales.SalesName, sales.WaGroupID)

		// 3. Dispatch Notification
		// Ideally we use a 'notifier' service, but for MVP we log it here.
		// If 2Chat/Group integration exists, we'd call it here.
		messageText := fmt.Sprintf(
			"🔥 *HOT LEAD DETECTED*\n\n" +
			"Name: %s\n" +
			"Phone: https://wa.me/%s\n" +
			"Interest: %s\n" +
			"Agent: %s\n" +
			"Assigned To: %s\n" +
			"Context: %s",
			lead.Name, lead.Phone, lead.Interest2, sales.SalesName, sales.SalesName, lead.Summary,
		)

		// TODO: Implement actual WA Group API call (e.g. via 2Chat or Gupshup partner API)
		log.Infof("TO WA GROUP [%s]: %s", sales.WaGroupID, messageText)

		// 4. Update Lead & Record Assignment
		err = db.Transaction(func(tx *gorm.DB) error {
			// Record the link
			if err := repo.NewSalesRepo(tx).RecordAssignment(ctx, sales.ID, lead.ID); err != nil {
				return err
			}

			// Patch lead status
			now := time.Now()
			patch := statemachine.Patch{
				SentToDev:       ptrBool(true),
				SentToWaGroupAt: &now,
			}
			audit := repo.AuditEntry{
				Actor:     "wa_group_dispatch",
				EventType: "wa_group_sent",
				Reason:    fmt.Sprintf("assigned to %s", sales.SalesName),
			}

			_, err = repo.NewLeadRepo(tx).TransitionTx(ctx, tx, lead.ID, lead.Version, patch, audit)
			return err
		})

		if err != nil {
			log.Errorf("Failed to finalize dispatch for lead %s: %v", lead.ID, err)
		} else {
			log.Infof("Lead %s successfully dispatched and assigned.", lead.ID)
		}
	}

	return nil
}

func ptrBool(v bool) *bool { return &v }
