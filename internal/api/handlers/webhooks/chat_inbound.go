package webhooks

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/workflow-builder/core/internal/model"
	"github.com/workflow-builder/core/internal/repo"
	"github.com/workflow-builder/core/internal/sdk"
	"github.com/workflow-builder/core/internal/statemachine"
	"github.com/workflow-builder/core/internal/workflows/leadflow"
	"github.com/workflow-builder/core/pkg/logger"
	"github.com/workflow-builder/core/pkg/phone"
	"gorm.io/gorm"
)

// ChatInboundHandler handles POST /api/webhooks/chat-inbound/:slug —
// forwarded Gupshup inbound messages from the n8n forwarder.
type ChatInboundHandler struct {
	db     *gorm.DB
	encKey []byte
	log    logger.Logger
}

// NewChatInboundHandler constructs a chat-inbound webhook handler.
func NewChatInboundHandler(db *gorm.DB, encKey []byte, log logger.Logger) *ChatInboundHandler {
	return &ChatInboundHandler{db: db, encKey: encKey, log: log}
}

// chatInboundBody is the payload shape n8n forwards. Keep flexible —
// n8n can send whatever column names it likes as long as these keys exist.
type chatInboundBody struct {
	ProviderMessageID string `json:"provider_message_id"`
	Phone             string `json:"phone"`
	Name              string `json:"name"`
	Content           string `json:"content"`
	Timestamp         string `json:"timestamp"`
}

// Handle processes one inbound chat message:
//  1. Verify HMAC
//  2. Normalize phone
//  3. Locate lead by (project, phone)
//  4. Insert lead_messages (dedupe by provider_message_id)
//  5. Transition via state machine (EventWAInbound)
//  6. Enqueue chatbot.process_turn task if not terminal
func (h *ChatInboundHandler) Handle(c *fiber.Ctx) error {
	slug := c.Params("slug")

	var biz model.Business
	if err := h.db.WithContext(c.Context()).First(&biz, "slug = ?", slug).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "unknown project"})
	}

	// HMAC verify against webhook_secret credential.
	secret, _ := sdk.GetCredential(h.db, biz.ID, "webhook_secret", h.encKey)
	if err := applyHMAC(c, secret); err != nil {
		return err
	}

	var body chatInboundBody
	if err := json.Unmarshal(c.Body(), &body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid json"})
	}

	// Normalize phone.
	canonicalPhone, err := phone.NormalizeID(body.Phone)
	if err != nil {
		h.log.Warnf("chat inbound: invalid phone %q: %v", body.Phone, err)
		return c.SendStatus(fiber.StatusOK)
	}

	leadRepo := repo.NewLeadRepo(h.db)
	lead, err := leadRepo.GetByPhone(c.Context(), biz.ID, canonicalPhone)
	if err != nil {
		// Unknown lead — log and ack; this is normal during ingestion
		// gaps and should not fail the webhook delivery.
		h.log.Infof("chat inbound: no matching lead for phone %s in project %s", canonicalPhone, slug)
		return c.SendStatus(fiber.StatusOK)
	}

	// Insert message with idempotency via provider_message_id unique index.
	msgRepo := repo.NewMessageRepo(h.db)
	msg := &model.LeadMessage{
		BusinessID:        biz.ID,
		LeadID:            lead.ID,
		Direction:         "inbound",
		Role:              "user",
		Content:           body.Content,
		ProviderMessageID: body.ProviderMessageID,
		CreatedAt:         time.Now(),
	}
	if err := msgRepo.Insert(c.Context(), msg); err != nil {
		if errors.Is(err, repo.ErrDuplicate) {
			// Replay — already processed.
			return c.SendStatus(fiber.StatusOK)
		}
		h.log.Errorf("chat inbound: insert message: %v", err)
		return c.SendStatus(fiber.StatusInternalServerError)
	}

	// Build the state machine event. IsFirstReply is derived from the
	// lead's current whatsapp_reply_at field.
	smEvent := statemachine.EventWAInbound{
		IsFirstReply: lead.WhatsappReplyAt == nil,
	}

	flowCfg, now, _ := loadProjectConfig(&biz)
	patch, commands, serr := statemachine.Transition(
		statemachine.LeadToStatemachine(*lead),
		smEvent,
		flowCfg,
		now,
		true,
	)
	if serr != nil {
		h.log.Errorf("chat inbound: statemachine: %v", serr)
		return c.SendStatus(fiber.StatusOK)
	}

	var updatedVersion int
	err = h.db.Transaction(func(tx *gorm.DB) error {
		if patch.IsEmpty() {
			updatedVersion = lead.Version
			return nil
		}
		audit := repo.AuditEntry{
			Actor:     "chat_inbound_webhook",
			EventType: "wa_inbound",
			Changes:   statemachine.PatchToMap(patch),
		}
		updated, err := leadRepo.TransitionTx(c.Context(), tx, lead.ID, lead.Version, patch, audit)
		if err != nil {
			return err
		}
		updatedVersion = updated.Version
		return nil
	})
	if err != nil {
		h.log.Errorf("chat inbound: transition: %v", err)
		return c.SendStatus(fiber.StatusOK)
	}

	// Enqueue CRM/other commands via the shared helper.
	leadflow.EnqueueCommands(c.Context(), biz.ID, lead.ID, updatedVersion, commands)

	// Always enqueue a chatbot turn unless the lead is in a chat-blocking
	// terminal state. terminal_responded does NOT block chatbot turns.
	if !lead.TerminalSpam && !lead.TerminalNotInterested {
		task, err := leadflow.NewChatbotTurnTask(leadflow.ChatbotTurnTaskPayload{
			LeadTaskPayload: leadflow.LeadTaskPayload{
				BusinessID: biz.ID,
				LeadID:     lead.ID,
			},
			MessageID: uint64(msg.ID),
		})
		if err == nil && leadflow.Deps() != nil && leadflow.Deps().Asynq != nil {
			_, _ = leadflow.Deps().Asynq.EnqueueContext(c.Context(), task)
		}
	}

	return c.SendStatus(fiber.StatusOK)
}
