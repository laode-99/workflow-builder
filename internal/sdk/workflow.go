package sdk

import (
	"context"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// ParamType defines how a workflow variable is collected in the UI
type ParamType string

const (
	ParamTypeString     ParamType = "string"
	ParamTypeCredential ParamType = "credential"
)

// Param defines a single input for a workflow
type Param struct {
	Key         string    `json:"key"`
	Type        ParamType `json:"type"`
	Description string    `json:"description,omitempty"`
	Optional    bool      `json:"optional,omitempty"`
	Integration string    `json:"integration,omitempty"` // For Credential types: e.g. "retell_ai"
}

// Step defines a logical node in a workflow flow
type Step struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Icon        string `json:"icon"` // Lucide icon name, e.g. "Database", "Phone"
	Description string `json:"description"`
}

// WorkflowDef maps a workflow to its UI requirements and handler function
type WorkflowDef struct {
	Name        string
	Description string
	Category    string // UI categorization: e.g. "ai_engine"
	Steps       []Step
	Params      []Param
	Handler     func(ctx context.Context, exec Execution) error
}

// WebhookDef maps an external callback to its handler
type WebhookDef struct {
	Path string
	Func func(ctx context.Context, db *gorm.DB, rdb *redis.Client, encKey []byte, query map[string]string, payload []byte) error
}

// Registry stores all registered workflows and webhooks in the platform
var Registry = struct {
	Workflows map[string]WorkflowDef
	Webhooks  map[string]WebhookDef
}{
	Workflows: make(map[string]WorkflowDef),
	Webhooks:  make(map[string]WebhookDef),
}

// RegisterWorkflow adds a workflow to the platform registry
func RegisterWorkflow(key string, definition WorkflowDef) {
	Registry.Workflows[key] = definition
}

// RegisterWebhook adds a webhook handler for external providers
func RegisterWebhook(key string, definition WebhookDef) {
	Registry.Webhooks[key] = definition
}
