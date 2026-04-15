package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Lead is the authoritative record of a lead being nurtured by the leadflow
// engine. Replaces the Google Sheet AI_Call_data row from the n8n reference.
//
// Uniqueness: (BusinessID, ExternalID). ExternalID is the CRM's opportunity
// identifier (LeadSquared ProspectOpportunityId). Internal references use
// the UUID primary key.
//
// Concurrency: writes must use optimistic locking via the Version column.
// See internal/repo.leadRepo.Transition for the canonical update path.
type Lead struct {
	ID                    uuid.UUID      `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	BusinessID            uuid.UUID      `gorm:"type:uuid;not null;index:idx_leads_business_external,priority:1;index:idx_leads_business_phone,priority:1" json:"business_id"`
	ExternalID            string         `gorm:"type:varchar(128);not null;index:idx_leads_business_external,priority:2,unique" json:"external_id"`
	Phone                 string         `gorm:"type:varchar(32);not null;index:idx_leads_business_phone,priority:2" json:"phone"`
	Name                  string         `gorm:"type:varchar(256)" json:"name"`
	Attempt               int            `gorm:"not null;default:0;index:idx_leads_attempt_cron,priority:1" json:"attempt"`
	CallDate              *time.Time     `gorm:"index:idx_leads_attempt_cron,priority:2" json:"call_date,omitempty"`
	DisconnectedReason    string         `gorm:"type:varchar(64)" json:"disconnected_reason,omitempty"`
	Interest              string         `gorm:"type:varchar(128)" json:"interest,omitempty"`
	Interest2             string         `gorm:"type:varchar(64)" json:"interest2,omitempty"`
	CustomerType          string         `gorm:"type:varchar(64)" json:"customer_type,omitempty"`
	SvsDate               *time.Time     `json:"svs_date,omitempty"`
	Summary               string         `gorm:"type:text;not null;default:''" json:"summary"`
	WhatsappSentAt        *time.Time     `json:"whatsapp_sent_at,omitempty"`
	WhatsappReplyAt       *time.Time     `json:"whatsapp_reply_at,omitempty"`
	SentToDev             bool           `gorm:"not null;default:false" json:"sent_to_dev"`
	SentToWAGroupAt       *time.Time     `json:"sent_to_wa_group_at,omitempty"`
	LeadSquaredPushedAt   *time.Time     `json:"leadsquared_pushed_at,omitempty"`
	ValidNumber           string         `gorm:"type:varchar(8)" json:"valid_number,omitempty"`
	NameAlertSent         bool           `gorm:"not null;default:false" json:"name_alert_sent"`
	TerminalInvalid       bool           `gorm:"not null;default:false" json:"terminal_invalid"`
	TerminalResponded     bool           `gorm:"not null;default:false" json:"terminal_responded"`
	TerminalNotInterested bool           `gorm:"not null;default:false" json:"terminal_not_interested"`
	TerminalSpam          bool           `gorm:"not null;default:false" json:"terminal_spam"`
	TerminalAgent         bool           `gorm:"not null;default:false" json:"terminal_agent"`
	TerminalCompleted     bool           `gorm:"not null;default:false" json:"terminal_completed"`
	Version               int            `gorm:"not null;default:0" json:"version"`
	CreatedAt             time.Time      `json:"created_at"`
	UpdatedAt             time.Time      `json:"updated_at"`
	DeletedAt             gorm.DeletedAt `gorm:"index" json:"-"`
}

// LeadMessage is an append-only chat message log entry. Replaces the
// Supabase `anandaya.conversation` column. Deduped via unique
// (BusinessID, ProviderMessageID) to absorb Gupshup webhook replays.
type LeadMessage struct {
	ID                uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	BusinessID        uuid.UUID `gorm:"type:uuid;not null;index:idx_lead_messages_dedupe,priority:1,unique" json:"business_id"`
	LeadID            uuid.UUID `gorm:"type:uuid;not null;index" json:"lead_id"`
	Direction         string    `gorm:"type:varchar(16);not null" json:"direction"` // inbound | outbound | system
	Role              string    `gorm:"type:varchar(16);not null" json:"role"`      // user | assistant | tool | system
	Content           string    `gorm:"type:text;not null" json:"content"`
	ProviderMessageID string    `gorm:"type:varchar(128);index:idx_lead_messages_dedupe,priority:2,unique" json:"provider_message_id,omitempty"`
	TokenUsage        string    `gorm:"type:jsonb" json:"token_usage,omitempty"`
	CreatedAt         time.Time `gorm:"index" json:"created_at"`
}

// LeadAudit records every state change to a Lead row. Written in the same
// transaction as the Lead update via leadRepo.Transition.
//
// Retention: rows older than 180 days are moved to lead_audits_archive by
// the system.audit_archiver singleton job.
type LeadAudit struct {
	ID            uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	BusinessID    uuid.UUID `gorm:"type:uuid;not null;index" json:"business_id"`
	LeadID        uuid.UUID `gorm:"type:uuid;not null;index" json:"lead_id"`
	Actor         string    `gorm:"type:varchar(64);not null" json:"actor"` // attempt_manager | retell_webhook | chatbot | admin_ui | ...
	EventType     string    `gorm:"type:varchar(64);not null" json:"event_type"`
	Changes       string    `gorm:"type:jsonb;not null;default:'{}'" json:"changes"`
	CorrelationID string    `gorm:"type:varchar(64)" json:"correlation_id,omitempty"`
	Reason        string    `gorm:"type:text" json:"reason,omitempty"`
	CreatedAt     time.Time `gorm:"index" json:"created_at"`
}

// CallEvent records a Retell webhook delivery. Deduped by
// (BusinessID, RetellCallID, Event) so duplicate webhook deliveries from
// Retell are absorbed without double-processing.
type CallEvent struct {
	ID                 uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	BusinessID         uuid.UUID `gorm:"type:uuid;not null;index:idx_call_events_dedupe,priority:1,unique" json:"business_id"`
	LeadID             uuid.UUID `gorm:"type:uuid;not null;index" json:"lead_id"`
	RetellCallID       string    `gorm:"type:varchar(128);not null;index:idx_call_events_dedupe,priority:2,unique" json:"retell_call_id"`
	Event              string    `gorm:"type:varchar(32);not null;index:idx_call_events_dedupe,priority:3,unique" json:"event"`
	Status             string    `gorm:"type:varchar(32)" json:"status,omitempty"`
	DisconnectedReason string    `gorm:"type:varchar(64)" json:"disconnected_reason,omitempty"`
	CallSummary        string    `gorm:"type:text" json:"call_summary,omitempty"`
	CustomAnalysis     string    `gorm:"type:jsonb" json:"custom_analysis,omitempty"`
	Payload            string    `gorm:"type:jsonb;not null" json:"payload"`
	CreatedAt          time.Time `gorm:"index" json:"created_at"`
}

// ChatbotState is the 1:1 companion of Lead holding chatbot-specific state
// (message count, spam flag, last chat timestamp, remarks summarization state).
type ChatbotState struct {
	LeadID         uuid.UUID  `gorm:"type:uuid;primaryKey" json:"lead_id"`
	BusinessID     uuid.UUID  `gorm:"type:uuid;not null;index" json:"business_id"`
	ChatTotal      int        `gorm:"not null;default:0" json:"chat_total"`
	SpamFlag       bool       `gorm:"not null;default:false" json:"spam_flag"`
	LastChatAt     *time.Time `json:"last_chat_at,omitempty"`
	ResetRemarks   bool       `gorm:"not null;default:false" json:"reset_remarks"`
	SessionKey     string     `gorm:"type:varchar(128)" json:"session_key,omitempty"`
	ChatbotRemarks string     `gorm:"type:text" json:"chatbot_remarks,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// SalesAssignment is a row in the per-project Sales/SPV roster used for
// round-robin dispatching of converted leads to developer WA groups.
type SalesAssignment struct {
	ID             uuid.UUID  `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	BusinessID     uuid.UUID  `gorm:"type:uuid;not null;index" json:"business_id"`
	SalesName      string     `gorm:"type:varchar(128);not null" json:"sales_name"`
	SpvName        string     `gorm:"type:varchar(128)" json:"spv_name,omitempty"`
	WAGroupID      string     `gorm:"type:varchar(128)" json:"wa_group_id,omitempty"`
	IsActive       bool       `gorm:"not null;default:true" json:"is_active"`
	LastAssignedAt *time.Time `json:"last_assigned_at,omitempty"`
	AssignCount    int        `gorm:"not null;default:0" json:"assign_count"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// LeadSalesAssignment records which Sales roster entry got which lead.
type LeadSalesAssignment struct {
	LeadID            uuid.UUID `gorm:"type:uuid;primaryKey" json:"lead_id"`
	SalesAssignmentID uuid.UUID `gorm:"type:uuid;not null;index" json:"sales_assignment_id"`
	AssignedAt        time.Time `json:"assigned_at"`
}

// ProjectPrompt stores a versioned system prompt per project per kind.
// Kinds: chatbot_system | chatbot_faq | chatbot_tool_instructions |
//
//	intent_classifier | spam_classifier | remarks_generator |
//	retell_agent_1 | retell_agent_3
//
// Only one row per (BusinessID, Kind) may be active at a time.
type ProjectPrompt struct {
	ID         uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	BusinessID uuid.UUID `gorm:"type:uuid;not null;index:idx_prompts_business_kind,priority:1" json:"business_id"`
	Kind       string    `gorm:"type:varchar(64);not null;index:idx_prompts_business_kind,priority:2" json:"kind"`
	Version    int       `gorm:"not null" json:"version"`
	Content    string    `gorm:"type:text;not null" json:"content"`
	IsActive   bool      `gorm:"not null;default:false" json:"is_active"`
	CreatedBy  string    `gorm:"type:varchar(128)" json:"created_by,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

// CRMSyncIntent is the outbox entry for a pending CRM write. Picked up by
// the system.crm_sync_outbox_poller singleton every 30 seconds.
// Status lifecycle: pending → in_progress → done | failed.
type CRMSyncIntent struct {
	ID          uuid.UUID  `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	BusinessID  uuid.UUID  `gorm:"type:uuid;not null;index" json:"business_id"`
	LeadID      uuid.UUID  `gorm:"type:uuid;not null;index" json:"lead_id"`
	Path        string     `gorm:"type:varchar(8);not null" json:"path"` // A | B
	Payload     string     `gorm:"type:jsonb;not null" json:"payload"`
	Status      string     `gorm:"type:varchar(16);not null;default:'pending';index:idx_crm_intents_status,priority:1" json:"status"`
	Attempts    int        `gorm:"not null;default:0" json:"attempts"`
	LastError   string     `gorm:"type:text" json:"last_error,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `gorm:"index:idx_crm_intents_status,priority:2" json:"updated_at"`
	ProcessedAt *time.Time `json:"processed_at,omitempty"`
}

// LeadAuditArchive mirrors LeadAudit's schema but holds rows older than 180 days.
type LeadAuditArchive struct {
	ID            uint64    `gorm:"primaryKey" json:"id"`
	BusinessID    uuid.UUID `gorm:"type:uuid;not null;index" json:"business_id"`
	LeadID        uuid.UUID `gorm:"type:uuid;not null;index" json:"lead_id"`
	Actor         string    `gorm:"type:varchar(64);not null" json:"actor"`
	EventType     string    `gorm:"type:varchar(64);not null" json:"event_type"`
	Changes       string    `gorm:"type:jsonb;not null;default:'{}'" json:"changes"`
	CorrelationID string    `gorm:"type:varchar(64)" json:"correlation_id,omitempty"`
	Reason        string    `gorm:"type:text" json:"reason,omitempty"`
	CreatedAt     time.Time `gorm:"index" json:"created_at"`
	ArchivedAt    time.Time `json:"archived_at"`
}
