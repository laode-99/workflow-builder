// Package repo contains data-access helpers for the leadflow engine.
//
// All writes to the Lead table go through LeadRepo.Transition, which applies
// an optimistic-locking update scoped by (id, version) and writes a matching
// LeadAudit row in the same DB transaction. This is the single concurrency
// primitive used by the engine.
package repo

import "errors"

// ErrVersionConflict is returned when an optimistic-lock UPDATE matches zero
// rows because the lead's version column has moved between read and write.
// Callers should re-read the lead and re-decide rather than blindly retry.
var ErrVersionConflict = errors.New("repo: lead version conflict (optimistic lock failed)")

// ErrNotFound wraps gorm.ErrRecordNotFound for repo-layer callers that don't
// want to import gorm directly.
var ErrNotFound = errors.New("repo: not found")

// ErrDuplicate is returned when a unique-constraint insert hits an existing
// row. Webhook handlers inspect this to distinguish "duplicate webhook
// replay" (expected, idempotent) from real errors.
var ErrDuplicate = errors.New("repo: duplicate row")
