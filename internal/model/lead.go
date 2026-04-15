package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Lead represents the central source of truth for a prospect, replacing the Google Sheet AI_Call_data.
type Lead struct {
	ID                    uuid.UUID      `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	BusinessID            uuid.UUID      `gorm:"type:uuid;not null;index:idx_leads_business_phone,priority:1;uniqueIndex:idx_leads_business_external,priority:1" json:"business_id"`
	ExternalID            string         `gorm:"type:varchar(128);not null;uniqueIndex:idx_leads_business_external,priority:2" json:"external_id"`
	Phone                 string         `gorm:"type:varchar(32);not null;index:idx_leads_business_phone,priority:2" json:"phone"` // Canonical format, e.g. "62xxx"
	Name                  string         `gorm:"type:varchar(256)" json:"name"`
	Attempt               int            `gorm:"not null;default:0;index:idx_leads_attempt_cron,priority:1" json:"attempt"`
	CallDate              *time.Time     `gorm:"index:idx_leads_attempt_cron,priority:2" json:"call_date,omitempty"` // Last call dispatch time
	DisconnectedReason    string         `gorm:"type:varchar(64)" json:"disconnected_reason,omitempty"`
	Interest              string         `gorm:"type:varchar(128)" json:"interest,omitempty"`
	Interest2             string         `gorm:"type:varchar(64)" json:"interest2,omitempty"`
	CustomerType          string         `gorm:"type:varchar(64)" json:"customer_type,omitempty"`
	SvsDate               *time.Time     `json:"svs_date,omitempty"`
	Summary               string         `gorm:"type:text;not null;default:''" json:"summary"`
	WhatsappSentAt        *time.Time     `json:"whatsapp_sent_at,omitempty"`
	WhatsappReplyAt       *time.Time     `json:"whatsapp_reply_at,omitempty"`
	SentToDev             bool           `gorm:"not null;default:false" json:"sent_to_dev"`
	SentToWaGroupAt       *time.Time     `json:"sent_to_wa_group_at,omitempty"`
	LeadsquaredPushedAt   *time.Time     `json:"leadsquared_pushed_at,omitempty"`
	ValidNumber           string         `gorm:"type:varchar(8)" json:"valid_number,omitempty"` // "Yes" | "No" | null
	NameAlertSent         bool           `gorm:"not null;default:false" json:"name_alert_sent"`
	
	// Terminal Flags
	TerminalInvalid       bool           `gorm:"not null;default:false" json:"terminal_invalid"`
	TerminalResponded     bool           `gorm:"not null;default:false" json:"terminal_responded"`
	TerminalNotInterested bool           `gorm:"not null;default:false" json:"terminal_not_interested"`
	TerminalSpam          bool           `gorm:"not null;default:false" json:"terminal_spam"`
	TerminalAgent         bool           `gorm:"not null;default:false" json:"terminal_agent"`
	TerminalCompleted     bool           `gorm:"not null;default:false" json:"terminal_completed"`
	
	// Distributed Concurrency & Audit
	Version               int            `gorm:"not null;default:0" json:"version"` // Optimistic locking
	CreatedAt             time.Time      `json:"created_at"`
	UpdatedAt             time.Time      `json:"updated_at"`
	DeletedAt             gorm.DeletedAt `gorm:"index" json:"-"`
	
	Business              Business       `gorm:"foreignKey:BusinessID" json:"business,omitempty"`
}

// ChatbotState isolated chatbot-specific state, 1:1 with Lead.
type ChatbotState struct {
	LeadID         uuid.UUID  `gorm:"type:uuid;primaryKey" json:"lead_id"`
	BusinessID     uuid.UUID  `gorm:"type:uuid;not null;index" json:"business_id"`
	ChatTotal      int        `gorm:"not null;default:0" json:"chat_total"`
	SpamFlag       bool       `gorm:"not null;default:false" json:"spam_flag"`
	LastChatAt     *time.Time `json:"last_chat_at"`
	ResetRemarks   bool       `gorm:"not null;default:false" json:"reset_remarks"`
	SessionKey     string     `gorm:"type:varchar(128)" json:"session_key"`
	ChatbotRemarks string     `gorm:"type:text" json:"chatbot_remarks"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`

	Lead           Lead       `gorm:"foreignKey:LeadID" json:"lead,omitempty"`
}
