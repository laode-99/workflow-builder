package model

import (
	"time"

	"github.com/google/uuid"
)

// LeadMessage stores append-only chat history (analogous to the old Supabase 'conversation' table)
type LeadMessage struct {
	ID                int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	BusinessID        uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_messages_idempotency" json:"business_id"`
	LeadID            uuid.UUID `gorm:"type:uuid;not null;index:idx_messages_lead_time" json:"lead_id"`
	Direction         string    `gorm:"type:varchar(16);not null" json:"direction"`  // inbound | outbound | system
	Role              string    `gorm:"type:varchar(16);not null" json:"role"`       // user | assistant | tool | system
	Content           string    `gorm:"type:text;not null" json:"content"`
	ProviderMessageID string    `gorm:"type:varchar(128);uniqueIndex:idx_messages_idempotency" json:"provider_message_id,omitempty"` // dedupe key for gupshup webhook replays
	TokenUsage        string    `gorm:"type:jsonb" json:"token_usage,omitempty"`
	CreatedAt         time.Time `gorm:"index:idx_messages_lead_time;not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	
	Lead              Lead      `gorm:"foreignKey:LeadID" json:"lead,omitempty"`
}
