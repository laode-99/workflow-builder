package api

import (
	"encoding/json"
	"log"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/workflow-builder/core/internal/integrations/webhook"
	"github.com/workflow-builder/core/internal/model"
	"github.com/workflow-builder/core/internal/repo"
	"github.com/workflow-builder/core/internal/statemachine"
)

func (h *Handler) leadflowRetell(c *fiber.Ctx) error {
	var body map[string]any
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid json"})
	}

	eventType, _ := body["event"].(string)
	callID, _ := body["call_id"].(string)
	if eventType == "" || callID == "" {
		return c.Status(400).JSON(fiber.Map{"error": "missing event or call_id"})
	}

	// 1. Resolve Lead via Retell Opportunity ID (metadata)
	// Gupshup/Retell usually pass the lead external_id in metadata
	var externalID string
	if metadata, ok := body["metadata"].(map[string]any); ok {
		externalID, _ = metadata["external_id"].(string)
	}

	if externalID == "" {
		// Fallback: search by recent call dispatch if metadata missing
		// For now, require metadata as it's cleaner.
		return c.Status(400).JSON(fiber.Map{"error": "missing external_id in metadata"})
	}

	// 2. Fetch Lead & Business
	var lead model.Lead
	if err := h.repo.db.First(&lead, "external_id = ?", externalID).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "lead not found"})
	}

	// 3. Idempotency Check
	callRepo := repo.NewCallEventRepo(h.repo.db)
	payloadB, _ := json.Marshal(body)
	event := &model.CallEvent{
		BusinessID:   lead.BusinessID,
		LeadID:       lead.ID,
		RetellCallID: callID,
		Event:        eventType,
		Payload:      string(payloadB),
	}

	if err := callRepo.Insert(c.Context(), event); err != nil {
		if err == repo.ErrDuplicate {
			return c.SendStatus(200) // Already processed
		}
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	// 4. Process Transition (Only for call_analyzed)
	if eventType == "call_analyzed" {
		var analysis map[string]any
		if a, ok := body["analysis"].(map[string]any); ok {
			analysis = a
		}

		// Extract state from analysis (Anandaya mapping)
		// custom_analysis -> interest
		interest, _ := analysis["custom_analysis"].(string)
		disconnected, _ := analysis["disconnect_reason"].(string)
		
		// Map back to statemachine Event
		ev := statemachine.EventCallAnalyzed{
			Attempt:            lead.Attempt,
			DisconnectedReason: disconnected,
			Interest:           interest,
			HasSufficientConvo: !strings.Contains(strings.ToLower(interest), "tidak ada percakapan"),
		}

		// Load Biz for config
		biz, _ := h.repo.GetBusiness(lead.BusinessID)
		var flowCfg statemachine.FlowConfig
		_ = json.Unmarshal([]byte(biz.Config), &flowCfg)

		patch, commands, err := statemachine.Transition(
			statemachine.LeadToStatemachine(lead), // Need helper
			ev,
			flowCfg,
			time.Now(),
			true, // assuming inbound call response is always "in hours" or business logic handles it
		)

		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "statemachine error: " + err.Error()})
		}

		// 5. Apply Patch
		audit := repo.AuditEntry{
			Actor:     "retell_webhook",
			EventType: "call_analyzed",
			Reason:    interest,
		}
		if _, err := repo.NewLeadRepo(h.repo.db).TransitionTx(c.Context(), h.repo.db, lead.ID, lead.Version, patch, audit); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "transition failed: " + err.Error()})
		}

		// 6. Enqueue Side Effects
		// TODO: Enqueue commands (CRM Sync, etc.)
		log.Printf("[LEADFLOW] Call analyzed for lead %s. Commands: %d", lead.ID, len(commands))
	}

	return c.JSON(fiber.Map{"ok": true})
}

func (h *Handler) chatInbound(c *fiber.Ctx) error {
	slug := c.Params("slug")
	business, err := h.repo.GetBusinessBySlug(slug)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "business not found"})
	}

	// 1. HMAC Verification
	var flowCfg statemachine.FlowConfig
	_ = json.Unmarshal([]byte(business.Config), &flowCfg)
	
	signature := c.Get("X-Signature")
	if !webhook.VerifyHMAC(c.Body(), flowCfg.WebhookSecret, signature) {
		return c.Status(401).JSON(fiber.Map{"error": "invalid signature"})
	}

	// 2. Parse Body (Gupshup Format)
	var payload map[string]any
	if err := c.BodyParser(&payload); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid body"})
	}

	// Deduce lead from phone
	phone, _ := payload["mobile"].(string) 
	if phone == "" {
		return c.Status(400).JSON(fiber.Map{"error": "missing phone"})
	}
	phone = strings.TrimPrefix(phone, "+")

	var lead model.Lead
	if err := h.repo.db.First(&lead, "business_id = ? AND phone = ?", business.ID, phone).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "lead not found for this project"})
	}

	// 3. Idempotency Check (Message ID)
	msgID, _ := payload["messageId"].(string)
	text, _ := payload["text"].(string)

	msgRepo := repo.NewMessageRepo(h.repo.db)
	err = msgRepo.Insert(c.Context(), &model.LeadMessage{
		BusinessID:        business.ID,
		LeadID:            lead.ID,
		Direction:         "inbound",
		Role:              "user",
		Content:           text,
		ProviderMessageID: msgID,
		CreatedAt:         time.Now(),
	})

	if err != nil {
		if err == repo.ErrDuplicate {
			return c.SendStatus(200)
		}
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	// 4. Transition
	isFirstReply := lead.WhatsappReplyAt == nil
	patch, _, _ := statemachine.Transition(
		statemachine.LeadToStatemachine(lead),
		statemachine.EventWAInbound{IsFirstReply: isFirstReply},
		flowCfg,
		time.Now(),
		true,
	)

	audit := repo.AuditEntry{
		Actor: "wa_webhook",
		EventType: "message_received",
	}
	_, _ = repo.NewLeadRepo(h.repo.db).TransitionTx(c.Context(), h.repo.db, lead.ID, lead.Version, patch, audit)

	// 5. Enqueue AI Turn Task
	// In Phase 6 we will implement the actual chatbot agent loop.
	// We'll enqueue an Asynq task here.
	log.Printf("[LEADFLOW] Received chat from %s. Enqueueing AI turn...", phone)
	
	return c.JSON(fiber.Map{"ok": true})
}
