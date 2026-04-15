package sdk

import (
	"context"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/workflow-builder/core/pkg/logger"
	"gorm.io/gorm"
)

// Execution represents the runtime context available to every workflow handler.
type Execution interface {
	ID() uuid.UUID
	BusinessID() uuid.UUID

	// Variables injected via UI configuration (workflow.variables JSON)
	GetVar(key string) string
	GetIntVar(key string, defaultVal int) int

	// Credential retrieval from encrypted vault
	GetCredential(integration string) (string, error)
	GetCredentialByID(id uuid.UUID) (string, error)
	GetCredentialByVar(key string) (string, error)

	// Access to internal infrastructure dependencies
	GetDB() *gorm.DB
	GetRedis() *redis.Client

	Logger() logger.Logger

	// Context and graceful stop
	Context() context.Context
	IsStopped() bool
	ShouldStop() bool
}
