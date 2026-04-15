package model

import (
	"time"

	"github.com/google/uuid"
)

// LeadAudit tracks explicit state transitions and reasonings.
type LeadAudit struct {
	ID            int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	BusinessID    uuid.UUID `gorm:"type:uuid;not null;index:idx_audits_project_time" json:"business_id"`
	LeadID        uuid.UUID `gorm:"type:uuid;not null;index:idx_audits_lead_time" json:"lead_id"`
	Actor         string    `gorm:"type:varchar(64);not null" json:"actor"`       // e.g. "attempt_manager", "chatbot", "admin_ui"
	EventType     string    `gorm:"type:varchar(64);not null" json:"event_type"`  // e.g. "attempt_advanced", "crm_sync"
	Changes       string    `gorm:"type:jsonb;not null" json:"changes"`           // JSON patch of what changed
	CorrelationID string    `gorm:"type:varchar(64)" json:"correlation_id,omitempty"`
	Reason        string    `gorm:"type:text" json:"reason,omitempty"`
	CreatedAt     time.Time `gorm:"index:idx_audits_project_time;index:idx_audits_lead_time;not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	
	Lead          Lead      `gorm:"foreignKey:LeadID" json:"lead,omitempty"`
}

// CallEvent tracks Retell outcomes and dedupes webhooks.
type CallEvent struct {
	ID                  int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	BusinessID          uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_call_events_dedupe" json:"business_id"`
	LeadID              uuid.UUID `gorm:"type:uuid;not null;index:idx_call_events_lead_time" json:"lead_id"`
	RetellCallID        string    `gorm:"type:varchar(128);not null;uniqueIndex:idx_call_events_dedupe" json:"retell_call_id"`
	Event               string    `gorm:"type:varchar(32);not null;uniqueIndex:idx_call_events_dedupe" json:"event"` // call_analyzed | call_ended | call_started
	Status              string    `gorm:"type:varchar(32)" json:"status"`
	DisconnectedReason  string    `gorm:"type:varchar(64)" json:"disconnected_reason"`
	CallSummary         string    `gorm:"type:text" json:"call_summary"`
	CustomAnalysis      string    `gorm:"type:jsonb" json:"custom_analysis"`
	Payload             string    `gorm:"type:jsonb;not null" json:"payload"` // raw webhook json
	CreatedAt           time.Time `gorm:"index:idx_call_events_lead_time;not null;default:CURRENT_TIMESTAMP" json:"created_at"`

	Lead                Lead      `gorm:"foreignKey:LeadID" json:"lead,omitempty"`
}

// Archive tables for the 6-month retention policy.
// They share the same structure but live in different tables.

type LeadArchive struct {
	Lead
}

type ChatMessageArchive struct {
	LeadMessage
}

type LeadAuditArchive struct {
	LeadAudit
}
