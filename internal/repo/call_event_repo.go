package repo

import (
	"context"
	"fmt"

	"github.com/workflow-builder/core/internal/model"
	"gorm.io/gorm"
)

// CallEventRepo handles idempotent Retell webhook inserts.
//
// Dedupe key: (business_id, retell_call_id, event). Duplicate webhook
// deliveries (e.g. call_analyzed arriving twice) hit the unique constraint
// and return ErrDuplicate so the caller can skip reprocessing.
type CallEventRepo struct {
	db *gorm.DB
}

func NewCallEventRepo(db *gorm.DB) *CallEventRepo {
	return &CallEventRepo{db: db}
}

// Insert saves a call event or returns ErrDuplicate if one already exists
// with the same (business_id, retell_call_id, event) tuple.
func (r *CallEventRepo) Insert(ctx context.Context, ev *model.CallEvent) error {
	err := r.db.WithContext(ctx).Create(ev).Error
	if err == nil {
		return nil
	}
	if isUniqueViolation(err) {
		return ErrDuplicate
	}
	return fmt.Errorf("insert call_event: %w", err)
}
