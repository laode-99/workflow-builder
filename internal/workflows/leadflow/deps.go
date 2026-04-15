package leadflow

import (
	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"
	"github.com/workflow-builder/core/pkg/logger"
	"gorm.io/gorm"
)

// Dependencies bundles the infrastructure handles shared by all leadflow
// cron handlers and per-lead Asynq task workers. The package-level
// singleton `deps` is set once at startup via SetDependencies so both
// sdk.Execution-driven cron handlers and raw Asynq task handlers can
// access the same clients.
type Dependencies struct {
	DB     *gorm.DB
	Redis  *redis.Client
	Asynq  *asynq.Client
	EncKey []byte
	Log    logger.Logger
}

// deps is the package-level dependency singleton.
var deps *Dependencies

// SetDependencies configures the leadflow package with shared infra handles.
// Must be called once at process startup before any handler runs.
func SetDependencies(d *Dependencies) {
	deps = d
}

// Deps returns the current dependencies, or nil if SetDependencies has not
// been called. Handlers should check for nil to fail fast.
func Deps() *Dependencies {
	return deps
}
