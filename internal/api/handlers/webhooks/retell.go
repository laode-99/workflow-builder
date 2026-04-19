package webhooks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/workflow-builder/core/internal/model"
	"github.com/workflow-builder/core/internal/repo"
	"github.com/workflow-builder/core/internal/sdk"
	"github.com/workflow-builder/core/internal/statemachine"
	"github.com/workflow-builder/core/internal/workflows/leadflow"
	"github.com/workflow-builder/core/pkg/logger"
	"gorm.io/gorm"
)

// RetellHandler handles POST /api/webhooks/retell/:slug — Retell's
// call_analyzed / call_ended / call_started webhook delivery.
type RetellHandler struct {
	db     *gorm.DB
	encKey []byte
	log    logger.Logger
}

// NewRetellHandler constructs a Retell webhook handler bound to shared infra.
func NewRetellHandler(db *gorm.DB, encKey []byte, log logger.Logger) *RetellHandler {
	return &RetellHandler{db: db, encKey: encKey, log: log}
}

// retellWebhookBody is the subset of Retell's payload we care about.
type retellWebhookBody struct {
	Event string `json:"event"`
	Call  struct {
		CallID              string                 `json:"call_id"`
		DisconnectionReason string                 `json:"disconnection_reason"`
		Metadata            map[string]string      `json:"metadata"`
		DynamicVariables    map[string]interface{} `json:"retell_llm_dynamic_variables"`
		CallAnalysis        struct {
			CallSummary        string                 `json:"call_summary"`
			CustomAnalysisData map[string]interface{} `json:"custom_analysis_data"`
		} `json:"call_analysis"`
	} `json:"call"`
}

// Handle is the Fiber handler. Flow:
//  1. Load project by slug
//  2. Verify HMAC against webhook_secret credential
//  3. Parse body; short-circuit if not "call_analyzed"
//  4. Insert call_event (unique constraint dedupe)
//  5. Locate the lead via metadata.lead_id or fallback lookup
//  6. Run state machine Transition
//  7. Apply patch inside a DB transaction
//  8. Enqueue side-effect commands after commit
func (h *RetellHandler) Handle(c *fiber.Ctx) error {
	slug := c.Params("slug")
	biz, err := h.loadProject(c.Context(), slug)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "unknown project"})
	}

	// HMAC verify.
	secret, _ := sdk.GetCredential(h.db, biz.ID, "webhook_secret", h.encKey)
	if err := applyHMAC(c, secret); err != nil {
		return err
	}

	var body retellWebhookBody
	if err := json.Unmarshal(c.Body(), &body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid json"})
	}

	// We only act on call_analyzed for state transitions; other events are
	// recorded for audit but skip the transition.
	actOnTransition := body.Event == "call_analyzed"

	leadID, ok := h.extractLeadID(c.Context(), biz.ID, body.Call.Metadata, body.Call.DynamicVariables)
	if !ok {
		h.log.Warnf("retell webhook: missing lead_id in metadata/dynamic_variables")
		return c.SendStatus(fiber.StatusOK)
	}

	// Dedupe via call_events unique index.
	eventRepo := repo.NewCallEventRepo(h.db)
	customJSON, _ := json.Marshal(body.Call.CallAnalysis.CustomAnalysisData)
	rawJSON, _ := json.Marshal(body)
	ev := &model.CallEvent{
		BusinessID:         biz.ID,
		LeadID:             leadID,
		RetellCallID:       body.Call.CallID,
		Event:              body.Event,
		DisconnectedReason: body.Call.DisconnectionReason,
		CallSummary:        body.Call.CallAnalysis.CallSummary,
		CustomAnalysis:     string(customJSON),
		Payload:            string(rawJSON),
		CreatedAt:          time.Now(),
	}
	if err := eventRepo.Insert(c.Context(), ev); err != nil {
		if errors.Is(err, repo.ErrDuplicate) {
			return c.SendStatus(fiber.StatusOK)
		}
		h.log.Errorf("retell webhook: insert call_event: %v", err)
		return c.SendStatus(fiber.StatusInternalServerError)
	}

	if !actOnTransition {
		return c.SendStatus(fiber.StatusOK)
	}

	// Load the lead to build the state machine event.
	leadRepo := repo.NewLeadRepo(h.db)
	lead, err := leadRepo.GetByID(c.Context(), leadID)
	if err != nil {
		h.log.Errorf("retell webhook: load lead %s: %v", leadID, err)
		return c.SendStatus(fiber.StatusOK)
	}

	// Parse custom_analysis fields into an EventCallAnalyzed.
	smEvent := buildCallAnalyzedEvent(body, lead.Attempt)

	flowCfg, now, _ := loadProjectConfig(biz)
	patch, commands, serr := statemachine.Transition(
		statemachine.LeadToStatemachine(*lead),
		smEvent,
		flowCfg,
		now,
		true, // webhooks always process, regardless of business hours
	)
	if serr != nil {
		h.log.Errorf("retell webhook: statemachine: %v", serr)
		return c.SendStatus(fiber.StatusOK)
	}

	var updatedVersion int
	err = h.db.Transaction(func(tx *gorm.DB) error {
		audit := repo.AuditEntry{
			Actor:     "retell_webhook",
			EventType: "call_analyzed",
			Changes:   statemachine.PatchToMap(patch),
			Reason:    body.Call.DisconnectionReason,
		}
		updated, err := leadRepo.TransitionTx(c.Context(), tx, lead.ID, lead.Version, patch, commands, audit)
		if err != nil {
			return err
		}
		updatedVersion = updated.Version
		return nil
	})
	if err != nil {
		h.log.Errorf("retell webhook: transition failed: %v", err)
		return c.SendStatus(fiber.StatusOK)
	}

	// Enqueue side effects after commit.
	leadflow.EnqueueCommands(c.Context(), biz.ID, lead.ID, updatedVersion, commands)
	return c.SendStatus(fiber.StatusOK)
}

func (h *RetellHandler) loadProject(ctx context.Context, slug string) (*model.Business, error) {
	var biz model.Business
	err := h.db.WithContext(ctx).First(&biz, "slug = ?", slug).Error
	if err != nil {
		return nil, err
	}
	return &biz, nil
}

// extractLeadID resolves the internal lead UUID from a Retell webhook.
// Preference order:
//  1. metadata.lead_id (set by our own dispatcher as a UUID)
//  2. dynamic_variables.leads_id (external CRM id — matched via leadRepo)
//
// Returns zero UUID + false if neither lookup succeeded.
func (h *RetellHandler) extractLeadID(ctx context.Context, businessID uuid.UUID, metadata map[string]string, dynVars map[string]interface{}) (uuid.UUID, bool) {
	if metadata != nil {
		if v, ok := metadata["lead_id"]; ok && v != "" {
			if u, err := uuid.Parse(v); err == nil {
				return u, true
			}
		}
	}
	// Fallback: Retell's Anandaya agent is configured with leads_id as a
	// dynamic variable carrying the LeadSquared opportunity id.
	if dynVars != nil {
		if raw, ok := dynVars["leads_id"]; ok {
			if externalID, ok := raw.(string); ok && externalID != "" {
				leadRepo := repo.NewLeadRepo(h.db)
				if lead, err := leadRepo.GetByExternalID(ctx, businessID, externalID); err == nil {
					return lead.ID, true
				}
			}
		}
	}
	return uuid.Nil, false
}

// buildCallAnalyzedEvent maps Retell's custom_analysis payload to the
// statemachine event shape, including the parsed site-visit date.
func buildCallAnalyzedEvent(body retellWebhookBody, currentAttempt int) statemachine.EventCallAnalyzed {
	custom := body.Call.CallAnalysis.CustomAnalysisData
	interest, _ := custom["site visit or no"].(string)
	customerType, _ := custom["customer_type"].(string)

	attempt := currentAttempt
	if dvAttempt, ok := body.Call.DynamicVariables["attempt"]; ok {
		if s, ok := dvAttempt.(string); ok {
			for _, ch := range s {
				if ch >= '0' && ch <= '9' {
					attempt = int(ch - '0')
					break
				}
			}
		}
	}

	// Parse "site visit date" — Retell's Anandaya agent emits this in
	// "M/D/YYYY H:MM:SS AM/PM" format, e.g. "4/15/2026 2:30:00 PM".
	var svsDate *time.Time
	if raw, ok := custom["site visit date"].(string); ok && raw != "" {
		layouts := []string{
			"1/2/2006 3:04:05 PM",
			"1/2/2006 15:04:05",
			"2006-01-02 15:04:05",
			"2006-01-02T15:04:05",
		}
		for _, layout := range layouts {
			if t, err := time.Parse(layout, raw); err == nil {
				svsDate = &t
				break
			}
		}
	}

	return statemachine.EventCallAnalyzed{
		Attempt:            attempt,
		DisconnectedReason: body.Call.DisconnectionReason,
		Interest:           interest,
		CustomerType:       customerType,
		SvsDate:            svsDate,
		HasSufficientConvo: interest != statemachine.InterestInsufficientConversation && interest != "",
	}
}

// loadProjectConfig parses the business's FlowConfig jsonb and returns it
// along with the current time.
func loadProjectConfig(biz *model.Business) (statemachine.FlowConfig, time.Time, error) {
	var cfg statemachine.FlowConfig
	if err := json.Unmarshal([]byte(biz.Config), &cfg); err != nil {
		return cfg, time.Now(), fmt.Errorf("parse config: %w", err)
	}
	if cfg.BusinessHours.Timezone != "" {
		if loc, err := time.LoadLocation(cfg.BusinessHours.Timezone); err == nil {
			return cfg, time.Now().In(loc), nil
		}
	}
	return cfg, time.Now(), nil
}
