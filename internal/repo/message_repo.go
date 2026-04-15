package repo

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/workflow-builder/core/internal/model"
	"gorm.io/gorm"
)

// MessageRepo handles append-only lead_messages inserts with provider-side
// idempotency (unique on business_id + provider_message_id).
type MessageRepo struct {
	db *gorm.DB
}

func NewMessageRepo(db *gorm.DB) *MessageRepo {
	return &MessageRepo{db: db}
}

// Insert saves a lead message. If the underlying insert violates the
// (business_id, provider_message_id) unique constraint (i.e. a Gupshup
// webhook replay), it returns ErrDuplicate so the webhook handler can
// return 200 without re-processing.
func (r *MessageRepo) Insert(ctx context.Context, msg *model.LeadMessage) error {
	err := r.db.WithContext(ctx).Create(msg).Error
	if err == nil {
		return nil
	}
	if isUniqueViolation(err) {
		return ErrDuplicate
	}
	return fmt.Errorf("insert lead_message: %w", err)
}

// ListRecent returns the last `limit` messages for a lead (newest first).
// Used by the chatbot agent to build the conversation window and by the
// timeline UI.
func (r *MessageRepo) ListRecent(ctx context.Context, leadID uuid.UUID, limit int) ([]model.LeadMessage, error) {
	var msgs []model.LeadMessage
	err := r.db.WithContext(ctx).
		Where("lead_id = ?", leadID).
		Order("created_at DESC").
		Limit(limit).
		Find(&msgs).Error
	if err != nil {
		return nil, fmt.Errorf("list lead messages: %w", err)
	}
	return msgs, nil
}

// ListWindow returns the last N user+assistant turns for a lead, filtered
// to roles the LLM understands. N is measured in messages (not turns), so
// a chatbot window of 4 turns means limit=8.
func (r *MessageRepo) ListWindow(ctx context.Context, leadID uuid.UUID, messageLimit int) ([]model.LeadMessage, error) {
	var msgs []model.LeadMessage
	err := r.db.WithContext(ctx).
		Where("lead_id = ? AND role IN ?", leadID, []string{"user", "assistant"}).
		Order("created_at DESC").
		Limit(messageLimit).
		Find(&msgs).Error
	if err != nil {
		return nil, fmt.Errorf("list message window: %w", err)
	}
	// Reverse to chronological order for prompt construction.
	for i, j := 0, len(msgs)-1; i < j; i, j = i+1, j-1 {
		msgs[i], msgs[j] = msgs[j], msgs[i]
	}
	return msgs, nil
}

// isUniqueViolation detects a Postgres unique-constraint error without
// importing the pgx driver directly (keeps the repo package driver-agnostic).
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "SQLSTATE 23505") ||
		strings.Contains(msg, "duplicate key") ||
		strings.Contains(msg, "unique constraint")
}
