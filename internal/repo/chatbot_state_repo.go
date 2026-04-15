package repo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/workflow-builder/core/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ChatbotStateRepo manages the 1:1 chatbot_states row per lead.
type ChatbotStateRepo struct {
	db *gorm.DB
}

func NewChatbotStateRepo(db *gorm.DB) *ChatbotStateRepo {
	return &ChatbotStateRepo{db: db}
}

// Upsert creates or updates the chatbot state for a lead, incrementing the
// message counter and refreshing timestamps. Safe to call from the chatbot
// agent at the end of each turn.
func (r *ChatbotStateRepo) RecordTurn(ctx context.Context, leadID, businessID uuid.UUID, sessionKey string, now time.Time) (*model.ChatbotState, error) {
	state := model.ChatbotState{
		LeadID:       leadID,
		BusinessID:   businessID,
		ChatTotal:    1,
		LastChatAt:   &now,
		ResetRemarks: false,
		SessionKey:   sessionKey,
	}
	err := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "lead_id"}},
			DoUpdates: clause.Assignments(map[string]any{
				"chat_total":    gorm.Expr("chatbot_states.chat_total + 1"),
				"last_chat_at":  now,
				"reset_remarks": false,
				"updated_at":    now,
			}),
		}).
		Create(&state).Error
	if err != nil {
		return nil, fmt.Errorf("upsert chatbot_state: %w", err)
	}

	// Re-read to get the authoritative counter value.
	var final model.ChatbotState
	if err := r.db.WithContext(ctx).First(&final, "lead_id = ?", leadID).Error; err != nil {
		return nil, fmt.Errorf("reread chatbot_state: %w", err)
	}
	return &final, nil
}

// Get returns the chatbot state for a lead, or ErrNotFound.
func (r *ChatbotStateRepo) Get(ctx context.Context, leadID uuid.UUID) (*model.ChatbotState, error) {
	var s model.ChatbotState
	err := r.db.WithContext(ctx).First(&s, "lead_id = ?", leadID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get chatbot_state: %w", err)
	}
	return &s, nil
}

// SetSpam marks the chatbot state as spam and is called by the spam classifier.
func (r *ChatbotStateRepo) SetSpam(ctx context.Context, leadID uuid.UUID) error {
	res := r.db.WithContext(ctx).
		Model(&model.ChatbotState{}).
		Where("lead_id = ?", leadID).
		Updates(map[string]any{
			"spam_flag":  true,
			"updated_at": time.Now(),
		})
	if res.Error != nil {
		return fmt.Errorf("set spam flag: %w", res.Error)
	}
	return nil
}

// WriteRemarks stores the generated remarks summary and marks reset_remarks=true.
func (r *ChatbotStateRepo) WriteRemarks(ctx context.Context, leadID uuid.UUID, remarks string) error {
	res := r.db.WithContext(ctx).
		Model(&model.ChatbotState{}).
		Where("lead_id = ?", leadID).
		Updates(map[string]any{
			"chatbot_remarks": remarks,
			"reset_remarks":   true,
			"updated_at":      time.Now(),
		})
	if res.Error != nil {
		return fmt.Errorf("write chatbot remarks: %w", res.Error)
	}
	return nil
}

// ListPendingRemarks returns chatbot states eligible for remarks generation
// (reset_remarks = false AND last_chat_at older than `delay` AND not null).
func (r *ChatbotStateRepo) ListPendingRemarks(ctx context.Context, businessID uuid.UUID, delay time.Duration, limit int) ([]model.ChatbotState, error) {
	cutoff := time.Now().Add(-delay)
	var states []model.ChatbotState
	err := r.db.WithContext(ctx).
		Where("business_id = ? AND reset_remarks = false AND last_chat_at IS NOT NULL AND last_chat_at <= ?", businessID, cutoff).
		Limit(limit).
		Find(&states).Error
	if err != nil {
		return nil, fmt.Errorf("list pending remarks: %w", err)
	}
	return states, nil
}
