package leadflow

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/workflow-builder/core/internal/model"
	"github.com/workflow-builder/core/internal/repo"
	"github.com/workflow-builder/core/internal/statemachine"
)

// fireInternalNameAlert sends a one-shot notification when a lead is
// ingested without a name. Mirrors the Anandaya n8n WF1-H behavior:
// notify operators, mark name_alert_sent=true, and leave the lead in the
// DB so it flows automatically once a human fills the name.
//
// Delivery: if the project has at least one active SalesAssignment with
// a wa_group_id, the alert goes to the first such group via Gupshup.
// Otherwise an audit-only entry is written for the admin dashboard.
//
// Either way name_alert_sent is flipped to true so future ingest passes
// don't re-alert. The attempt manager's name != '' filter keeps the
// lead out of escalation until a human updates the name — at which
// point the next cron tick picks it up normally.
func fireInternalNameAlert(ctx context.Context, lead *model.Lead) error {
	if lead == nil || deps == nil {
		return fmt.Errorf("internal alert: deps or lead nil")
	}

	var biz model.Business
	_ = deps.DB.WithContext(ctx).First(&biz, "id = ?", lead.BusinessID).Error
	bizName := biz.Name
	if bizName == "" {
		bizName = "Project"
	}
	msg := fmt.Sprintf(
		"⚠️ Name Missing\n\n"+
			"Project: %s\n"+
			"Phone: +%s\n"+
			"External ID: %s\n\n"+
			"Namanya masih kosong, tolong bantu diisi ya. Lead akan otomatis diproses setelah nama ditambahkan.",
		bizName, lead.Phone, lead.ExternalID,
	)

	// Internal alerts go to the team via 2Chat (not Gupshup).
	// Gupshup is customer-facing only.
	salesRepo := repo.NewSalesRepo(deps.DB)
	list, _ := salesRepo.ListActiveAssignments(ctx, lead.BusinessID)
	sent := false
	for _, sa := range list {
		if sa.WaGroupID == "" {
			continue
		}
		if err := sendToInternalGroup(ctx, lead.BusinessID, sa.WaGroupID, msg); err == nil {
			sent = true
			break
		}
	}

	flipped := true
	patch := statemachine.Patch{NameAlertSent: &flipped}
	audit := repo.AuditEntry{
		Actor:     "ingest.internal_alert",
		EventType: "name_empty_alert",
		Changes: map[string]any{
			"sent_via_wa": sent,
		},
		Reason: "lead ingested with empty name; waiting for human name entry",
	}
	leadRepo := repo.NewLeadRepo(deps.DB)
	if _, err := leadRepo.Transition(ctx, lead.ID, lead.Version, patch, audit); err != nil {
		return fmt.Errorf("mark name_alert_sent: %w", err)
	}
	deps.Log.Infof("internal name alert fired: lead=%s sent_via_wa=%v", lead.ID, sent)
	return nil
}

// Explicit use so the uuid import isn't considered unused once we expand
// this file later with type-qualified helpers. (Keeps `go build` happy.)
var _ = uuid.Nil
