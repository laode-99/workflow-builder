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

		// 3a. Valid-number gate via 2Chat — skip the handoff if the
		// customer's phone isn't on WhatsApp. Anandaya n8n does this
		// check before notifying the developer team to avoid cold-email
		// clutter. On lookup failure we proceed anyway (fail-open so a
		// flaky 2chat doesn't block the pipeline).
		if onWA, err := checkValidWANumber(ctx, bizID, lead.Phone); err == nil && !onWA {
			log.Warnf("Lead %s phone %s not on WhatsApp; skipping WA group handoff.", lead.ID, lead.Phone)
			// Mark as dispatched anyway so the lead isn't retried on every cron.
			now := time.Now()
			_, _ = leadRepo.Transition(ctx, lead.ID, lead.Version, statemachine.Patch{
				SentToDev:       ptrBool(true),
				SentToWaGroupAt: &now,
			}, repo.AuditEntry{
				Actor:     "wa_group_dispatch",
				EventType: "wa_group_skipped_no_whatsapp",
				Reason:    "phone not registered on whatsapp",
			})
			continue
		}

		// 3b. Build the handoff message.
		messageText := fmt.Sprintf(
			"🔥 *HOT LEAD DETECTED*\n\n"+
				"Name: %s\n"+
				"Phone: https://wa.me/%s\n"+
				"Interest: %s\n"+
				"Assigned To: %s\n"+
				"Context: %s",
			lead.Name, lead.Phone, lead.Interest2, sales.SalesName, lead.Summary,
		)

		// 3c. Send via 2Chat (internal team channel — not Gupshup).
		if err := sendToInternalGroup(ctx, bizID, sales.WaGroupID, messageText); err != nil {
			log.Errorf("2chat send failed for lead %s → group %s: %v", lead.ID, sales.WaGroupID, err)
			// Leave the lead un-marked so the next cron tick retries.
			continue
		}
		log.Infof("Lead %s notified to WA group %s via 2Chat", lead.ID, sales.WaGroupID)

		// 4. Update Lead & Record Assignment ONLY after successful send.
		err = db.Transaction(func(tx *gorm.DB) error {
			if err := repo.NewSalesRepo(tx).RecordAssignment(ctx, sales.ID, lead.ID); err != nil {
				return err
			}
			now := time.Now()
			patch := statemachine.Patch{
				SentToDev:       ptrBool(true),
				SentToWaGroupAt: &now,
			}
			audit := repo.AuditEntry{
				Actor:     "wa_group_dispatch",
				EventType: "wa_group_sent",
				Reason:    fmt.Sprintf("assigned to %s via 2chat", sales.SalesName),
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
