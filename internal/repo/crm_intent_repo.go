package repo

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/workflow-builder/core/internal/model"
	"gorm.io/gorm"
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

// ClaimBatch atomically marks up to `limit` pending intents as in_progress
// and returns them for processing. Stale in_progress rows (older than
// `staleAfter`) are reclaimed — this covers worker crashes mid-flight.
func (r *CRMIntentRepo) ClaimBatch(ctx context.Context, limit int, staleAfter time.Duration) ([]model.CRMSyncIntent, error) {
	var claimed []model.CRMSyncIntent
	now := time.Now()
	cutoff := now.Add(-staleAfter)

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Find candidate IDs.
		var ids []uuid.UUID
		subErr := tx.Model(&model.CRMSyncIntent{}).
			Where("status = ? OR (status = ? AND updated_at < ?)", "pending", "in_progress", cutoff).
			Order("created_at ASC").
			Limit(limit).
			Pluck("id", &ids).Error
		if subErr != nil {
			return fmt.Errorf("pluck pending crm intents: %w", subErr)
		}
		if len(ids) == 0 {
			return nil
		}

		// Claim them.
		res := tx.Model(&model.CRMSyncIntent{}).
			Where("id IN ?", ids).
			Updates(map[string]any{
				"status":     "in_progress",
				"updated_at": now,
			})
		if res.Error != nil {
			return fmt.Errorf("claim crm intents: %w", res.Error)
		}

		// Load the claimed rows for processing.
		if err := tx.Where("id IN ?", ids).Find(&claimed).Error; err != nil {
			return fmt.Errorf("load claimed crm intents: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return claimed, nil
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

// MarkFailed marks an intent as permanently failed and records the error.
func (r *CRMIntentRepo) MarkFailed(ctx context.Context, id uuid.UUID, errMsg string) error {
	now := time.Now()
	res := r.db.WithContext(ctx).
		Model(&model.CRMSyncIntent{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"status":       "failed",
			"last_error":   errMsg,
			"processed_at": now,
			"updated_at":   now,
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
