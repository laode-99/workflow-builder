package repo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/workflow-builder/core/internal/model"
	"github.com/workflow-builder/core/internal/statemachine"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// LeadRepo is the data-access layer for lead lifecycle operations.
type LeadRepo struct {
	db *gorm.DB
}

// NewLeadRepo constructs a LeadRepo bound to the given GORM handle.
func NewLeadRepo(db *gorm.DB) *LeadRepo {
	return &LeadRepo{db: db}
}

// DB exposes the underlying GORM handle for callers that need to open their
// own transactions (e.g. the attempt-manager worker which reads + transitions
// a batch of leads in one tx).
func (r *LeadRepo) DB() *gorm.DB {
	return r.db
}

// AuditEntry describes the audit row to be written in the same transaction
// as a Transition. Fields map directly to model.LeadAudit columns.
type AuditEntry struct {
	Actor         string
	EventType     string
	Changes       map[string]any
	CorrelationID string
	Reason        string
}

// Create inserts a brand-new lead row. Used by leadflow.ingest.
func (r *LeadRepo) Create(ctx context.Context, lead *model.Lead) error {
	if err := r.db.WithContext(ctx).Create(lead).Error; err != nil {
		return fmt.Errorf("create lead: %w", err)
	}
	return nil
}

// GetByID loads a single lead by primary key.
func (r *LeadRepo) GetByID(ctx context.Context, id uuid.UUID) (*model.Lead, error) {
	var lead model.Lead
	err := r.db.WithContext(ctx).First(&lead, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get lead: %w", err)
	}
	return &lead, nil
}

// GetByExternalID loads a lead by its CRM-side opportunity identifier
// within a given project.
func (r *LeadRepo) GetByExternalID(ctx context.Context, businessID uuid.UUID, externalID string) (*model.Lead, error) {
	var lead model.Lead
	err := r.db.WithContext(ctx).
		First(&lead, "business_id = ? AND external_id = ?", businessID, externalID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get lead by external id: %w", err)
	}
	return &lead, nil
}

// GetByPhone loads the most recent lead matching a phone number within a
// project. Used by the chat-inbound webhook to locate the lead by the
// sender's phone.
func (r *LeadRepo) GetByPhone(ctx context.Context, businessID uuid.UUID, phone string) (*model.Lead, error) {
	var lead model.Lead
	err := r.db.WithContext(ctx).
		Order("created_at DESC").
		First(&lead, "business_id = ? AND phone = ?", businessID, phone).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get lead by phone: %w", err)
	}
	return &lead, nil
}

// GetDueForDispatch selects a batch of leads eligible for cron-driven
// escalation, using FOR UPDATE SKIP LOCKED so multiple worker replicas do
// not pick the same rows. The filter matches non-terminal leads in
// actionable attempt levels.
//
// The caller MUST pass an open *gorm.DB transaction (tx) — the row locks
// are only valid until that transaction commits. Typical usage:
//
//	db.Transaction(func(tx *gorm.DB) error {
//	    leads, err := repo.GetDueForDispatch(ctx, tx, bizID, 100)
//	    for _, lead := range leads {
//	        // Evaluate state machine and call Transition(...) inside tx
//	    }
//	    return nil
//	})
func (r *LeadRepo) GetDueForDispatch(ctx context.Context, tx *gorm.DB, businessID uuid.UUID, limit int) ([]model.Lead, error) {
	var leads []model.Lead
	err := tx.WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
		Where("business_id = ? AND deleted_at IS NULL", businessID).
		Where("NOT (terminal_invalid OR terminal_responded OR terminal_not_interested OR terminal_spam OR terminal_agent OR terminal_completed)").
		Where("attempt IN ?", []int{0, 2, 3, 4}).
		// Empty-name gate: leads ingested without a name are held until a
		// human fills it in. The ingest flow fires an internal alert for
		// these; once the name is updated, the next cron tick picks them up.
		Where("name IS NOT NULL AND name <> ''").
		Order("updated_at ASC").
		Limit(limit).
		Find(&leads).Error
	if err != nil {
		return nil, fmt.Errorf("select due leads: %w", err)
	}
	return leads, nil
}

// ListNameless returns leads whose name is still empty and whose internal
// alert has not yet been fired. The internal alert job (or ingest flow)
// processes these and flips name_alert_sent after notifying operators.
func (r *LeadRepo) ListNameless(ctx context.Context, businessID uuid.UUID, limit int) ([]model.Lead, error) {
	var leads []model.Lead
	err := r.db.WithContext(ctx).
		Where("business_id = ? AND deleted_at IS NULL", businessID).
		Where("(name IS NULL OR name = '')").
		Where("name_alert_sent = false").
		Limit(limit).
		Find(&leads).Error
	if err != nil {
		return nil, fmt.Errorf("list nameless leads: %w", err)
	}
	return leads, nil
}

// GetPendingWADispatch selects leads that have were classified as hot/callback 
// by the chatbot but haven't been sent to a sales team WA group yet.
func (r *LeadRepo) GetPendingWADispatch(ctx context.Context, businessID uuid.UUID, limit int) ([]model.Lead, error) {
	var leads []model.Lead
	err := r.db.WithContext(ctx).
		Where("business_id = ? AND sent_to_dev = false AND (interest2 = ? OR interest2 = ?)", 
			businessID, "Callback", "Agent").
		Where("terminal_spam = false").
		Limit(limit).
		Find(&leads).Error
	if err != nil {
		return nil, fmt.Errorf("select pending wa dispatch: %w", err)
	}
	return leads, nil
}

// Transition applies a state-machine Patch to a lead atomically, using
// optimistic locking on the Version column. On version mismatch it returns
// ErrVersionConflict — the caller should re-read and re-decide rather than
// blindly retry.
//
// An audit row is written in the same DB transaction, so either both
// succeed or neither does.
//
// Side-effect commands from the state machine are NOT enqueued here. The
// caller enqueues them only after Transition returns successfully — this
// is the enqueue-after-commit pattern that keeps the system consistent
// when Redis is transiently unavailable.
func (r *LeadRepo) Transition(
	ctx context.Context,
	leadID uuid.UUID,
	expectedVersion int,
	patch statemachine.Patch,
	audit AuditEntry,
) (*model.Lead, error) {
	return r.TransitionTx(ctx, r.db, leadID, expectedVersion, patch, audit)
}

// TransitionTx is Transition scoped to a caller-provided transaction,
// used when the caller is already inside a `db.Transaction(func(tx) ...)`
// block (e.g. the attempt-manager's SELECT FOR UPDATE batch loop).
func (r *LeadRepo) TransitionTx(
	ctx context.Context,
	tx *gorm.DB,
	leadID uuid.UUID,
	expectedVersion int,
	patch statemachine.Patch,
	audit AuditEntry,
) (*model.Lead, error) {
	// Short-circuit: if caller passes an empty patch and no audit event,
	// just return the current row without touching it.
	if patch.IsEmpty() && audit.EventType == "" {
		var lead model.Lead
		if err := tx.WithContext(ctx).First(&lead, "id = ?", leadID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, ErrNotFound
			}
			return nil, fmt.Errorf("reread lead: %w", err)
		}
		return &lead, nil
	}

	var result *model.Lead

	txFn := func(tx *gorm.DB) error {
		if !patch.IsEmpty() {
			updates := patchToMap(patch)
			updates["version"] = gorm.Expr("version + 1")
			updates["updated_at"] = time.Now()

			res := tx.Model(&model.Lead{}).
				Where("id = ? AND version = ? AND deleted_at IS NULL", leadID, expectedVersion).
				Updates(updates)
			if res.Error != nil {
				return fmt.Errorf("update lead: %w", res.Error)
			}
			if res.RowsAffected == 0 {
				return ErrVersionConflict
			}
		}

		var lead model.Lead
		if err := tx.First(&lead, "id = ?", leadID).Error; err != nil {
			return fmt.Errorf("reread lead: %w", err)
		}
		result = &lead

		if audit.EventType != "" {
			changesJSON, err := marshalJSON(audit.Changes)
			if err != nil {
				return fmt.Errorf("marshal audit changes: %w", err)
			}
			auditRow := &model.LeadAudit{
				BusinessID:    lead.BusinessID,
				LeadID:        lead.ID,
				Actor:         audit.Actor,
				EventType:     audit.EventType,
				Changes:       changesJSON,
				CorrelationID: audit.CorrelationID,
				Reason:        audit.Reason,
			}
			if err := tx.Create(auditRow).Error; err != nil {
				return fmt.Errorf("insert audit: %w", err)
			}
		}
		return nil
	}

	// If the caller passed our own db (i.e. Transition, not TransitionTx),
	// open a new transaction. Otherwise reuse theirs.
	if tx == r.db {
		err := r.db.WithContext(ctx).Transaction(txFn)
		if err != nil {
			return nil, err
		}
	} else {
		if err := txFn(tx.WithContext(ctx)); err != nil {
			return nil, err
		}
	}
	return result, nil
}

// patchToMap converts a non-nil patch into a gorm-friendly map[string]any
// for use with .Updates(). Only fields whose pointers are non-nil are
// included, so a patch that only touches `attempt` won't clobber other
// columns.
func patchToMap(p statemachine.Patch) map[string]any {
	m := make(map[string]any, 14)
	if p.Attempt != nil {
		m["attempt"] = *p.Attempt
	}
	if p.CallDate != nil {
		m["call_date"] = *p.CallDate
	}
	if p.WhatsappSentAt != nil {
		m["whatsapp_sent_at"] = *p.WhatsappSentAt
	}
	if p.WhatsappReplyAt != nil {
		m["whatsapp_reply_at"] = *p.WhatsappReplyAt
	}
	if p.DisconnectedReason != nil {
		m["disconnected_reason"] = *p.DisconnectedReason
	}
	if p.Interest != nil {
		m["interest"] = *p.Interest
	}
	if p.Interest2 != nil {
		m["interest2"] = *p.Interest2
	}
	if p.CustomerType != nil {
		m["customer_type"] = *p.CustomerType
	}
	if p.TerminalInvalid != nil {
		m["terminal_invalid"] = *p.TerminalInvalid
	}
	if p.TerminalResponded != nil {
		m["terminal_responded"] = *p.TerminalResponded
	}
	if p.TerminalNotInterested != nil {
		m["terminal_not_interested"] = *p.TerminalNotInterested
	}
	if p.TerminalSpam != nil {
		m["terminal_spam"] = *p.TerminalSpam
	}
	if p.TerminalAgent != nil {
		m["terminal_agent"] = *p.TerminalAgent
	}
	if p.TerminalCompleted != nil {
		m["terminal_completed"] = *p.TerminalCompleted
	}
	if p.Summary != nil {
		m["summary"] = *p.Summary
	}
	if p.SentToDev != nil {
		m["sent_to_dev"] = *p.SentToDev
	}
	if p.SentToWaGroupAt != nil {
		m["sent_to_wa_group_at"] = *p.SentToWaGroupAt
	}
	return m
}

func marshalJSON(m map[string]any) (string, error) {
	if m == nil {
		return "{}", nil
	}
	b, err := json.Marshal(m)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
