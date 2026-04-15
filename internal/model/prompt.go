package model

import (
	"time"

	"github.com/google/uuid"
)

// ProjectPrompt stores versioned configuration for standard LLM prompts.
// Ensures that tweaks to chatbots or intent classifiers can be rolled back.
type ProjectPrompt struct {
	ID         uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	BusinessID uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_prompts_dedupe" json:"business_id"`
	Kind       string    `gorm:"type:varchar(64);not null;uniqueIndex:idx_prompts_dedupe" json:"kind"`
	Version    int       `gorm:"not null;uniqueIndex:idx_prompts_dedupe" json:"version"`
	Content    string    `gorm:"type:text;not null" json:"content"`
	IsActive   bool      `gorm:"not null;default:false" json:"is_active"`
	CreatedBy  *string   `gorm:"type:varchar(128)" json:"created_by"`
	CreatedAt  time.Time `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`

	Business   Business  `gorm:"foreignKey:BusinessID" json:"business,omitempty"`
}
