package model

import (
	"time"

	"github.com/google/uuid"
)

// CRMSyncIntent serves as an outbox for ensuring CRM writes happen exactly once,
// tracking retries and failures since LeadSquared isn't idempotent.
type CRMSyncIntent struct {
	ID          uuid.UUID  `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	BusinessID  uuid.UUID  `gorm:"type:uuid;not null;index" json:"business_id"`
	LeadID      uuid.UUID  `gorm:"type:uuid;not null;index" json:"lead_id"`
	Path        string     `gorm:"type:varchar(8);not null" json:"path"` // e.g. "A" or "B"
	Payload     string     `gorm:"type:jsonb;not null" json:"payload"`
	Status      string     `gorm:"type:varchar(16);not null;default:'pending';index:idx_crm_intents_pending" json:"status"` // pending | in_progress | done | failed
	Attempts    int        `gorm:"not null;default:0" json:"attempts"`
	LastError   *string    `gorm:"type:text" json:"last_error"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `gorm:"index:idx_crm_intents_pending" json:"updated_at"`
	ProcessedAt *time.Time `json:"processed_at"`

	Lead        Lead       `gorm:"foreignKey:LeadID" json:"lead,omitempty"`
	Business    Business   `gorm:"foreignKey:BusinessID" json:"business,omitempty"`
}
