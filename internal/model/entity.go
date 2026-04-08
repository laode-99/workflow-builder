package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Business represents a tenant / company
type Business struct {
	ID        uuid.UUID      `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	Name      string         `gorm:"not null" json:"name"`
	Slug      string         `gorm:"uniqueIndex;not null" json:"slug"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

// Credential holds AES-encrypted API keys or OAuth tokens scoped to a Business
type Credential struct {
	ID          uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	BusinessID  uuid.UUID `gorm:"type:uuid;not null;index" json:"business_id"`
	Label       string    `gorm:"not null" json:"label"`       // human-friendly name e.g. "Retell Production Key"
	Integration string    `gorm:"not null" json:"integration"` // e.g. "retell_ai", "google_sheets"
	IsGlobal    bool      `gorm:"default:false" json:"is_global"`
	DataEnc     []byte    `gorm:"not null" json:"-"` // never exposed via API
	CreatedAt   time.Time `json:"created_at"`
}

// Workflow ties an SDK-registered workflow signature to a specific Business
type Workflow struct {
	ID          uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	BusinessID  uuid.UUID `gorm:"type:uuid;not null;index" json:"business_id"`
	Signature   string    `gorm:"not null" json:"signature"`    // must match sdk.Registry key
	Alias       string    `gorm:"not null" json:"alias"`        // UI display name
	IsActive    bool      `gorm:"default:false" json:"is_active"`
	TriggerCron string    `json:"trigger_cron,omitempty"`
	StopTime    string    `json:"stop_time,omitempty"` // e.g. "21:00" Jakarta time
	Variables   string    `gorm:"type:jsonb;default:'{}'" json:"variables"` // runtime config JSON
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Execution tracks a single run of a workflow
type Execution struct {
	ID          uuid.UUID  `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	WorkflowID  uuid.UUID  `gorm:"type:uuid;not null;index" json:"workflow_id"`
	ExternalID  string     `gorm:"index" json:"external_id,omitempty"` // For mapping to n8n execution ID
	Status      string     `gorm:"default:'queued'" json:"status"` // queued | running | completed | failed | stopped
	ErrorMsg    string     `json:"error_msg,omitempty"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`

	Workflow Workflow `gorm:"foreignKey:WorkflowID" json:"workflow,omitempty"`
}

// ExecutionLog stores individual log lines emitted during execution
type ExecutionLog struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	ExecutionID uuid.UUID `gorm:"type:uuid;not null;index" json:"execution_id"`
	Level       string    `gorm:"not null" json:"level"` // INFO | WARN | ERROR
	Message     string    `gorm:"not null" json:"message"`
	CreatedAt   time.Time `json:"created_at"`
}
