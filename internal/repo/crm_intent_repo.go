package repo

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/workflow-builder/core/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// CRMIntentRepo manages crm_sync_intents — the outbox used to guarantee
// eventually-consistent CRM writes even when LeadSquared is unavailable.
type CRMIntentRepo struct {
	db *gorm.DB
}

func NewCRMIntentRepo(db *gorm.DB) *CRMIntentRepo {
	return &CRMIntentRepo{db: db}
}

// CreateTx inserts a pending CRM intent inside the caller's transaction.
// Used by webhook handlers and leadflow workers that want the intent
// persisted atomically with the lead update that triggered it.
func (r *CRMIntentRepo) CreateTx(ctx context.Context, tx *gorm.DB, intent *model.CRMSyncIntent) error {
	if intent.Status == "" {
		intent.Status = "pending"
	}
	if err := tx.WithContext(ctx).Create(intent).Error; err != nil {
		return fmt.Errorf("create crm_sync_intent: %w", err)
	}
	return nil
}

// GetPendingForProcessing fetches intents that are pending or have transient errors,
// using SKIP LOCKED to ensure atomic processing across multiple worker instances.
func (r *CRMIntentRepo) GetPendingForProcessing(ctx context.Context, limit int) ([]model.CRMSyncIntent, error) {
	var items []model.CRMSyncIntent
	
	err := r.db.WithContext(ctx).
		Transaction(func(tx *gorm.DB) error {
			return tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
				Where("status IN ? AND attempts < ?", []string{"pending", "failed"}, 5).
				Order("created_at asc").
				Limit(limit).
				Find(&items).Error
		})
	
	return items, err
}

// MarkDone marks an intent as successfully processed.
func (r *CRMIntentRepo) MarkDone(ctx context.Context, id uuid.UUID) error {
	now := time.Now()
	res := r.db.WithContext(ctx).
		Model(&model.CRMSyncIntent{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"status":       "done",
			"processed_at": now,
			"updated_at":   now,
		})
	if res.Error != nil {
		return fmt.Errorf("mark crm intent done: %w", res.Error)
	}
	return nil
}

// MarkFailed records a failure for an intent and increments the attempt counter.
func (r *CRMIntentRepo) MarkFailed(ctx context.Context, id uuid.UUID, errMsg string) error {
	res := r.db.WithContext(ctx).
		Model(&model.CRMSyncIntent{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"status":     "failed",
			"last_error": errMsg,
			"attempts":   gorm.Expr("attempts + 1"),
			"updated_at": time.Now(),
		})
	if res.Error != nil {
		return fmt.Errorf("mark crm intent failed: %w", res.Error)
	}
	return nil
}

// MarkRetryable returns an in_progress intent to pending so the next poll
// picks it up. Used after a transient failure.
func (r *CRMIntentRepo) MarkRetryable(ctx context.Context, id uuid.UUID, errMsg string) error {
	now := time.Now()
	res := r.db.WithContext(ctx).
		Model(&model.CRMSyncIntent{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"status":     "pending",
			"attempts":   gorm.Expr("attempts + 1"),
			"last_error": errMsg,
			"updated_at": now,
		})
	if res.Error != nil {
		return fmt.Errorf("mark crm intent retryable: %w", res.Error)
	}
	return nil
}
