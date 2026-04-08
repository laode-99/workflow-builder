package sdk

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/workflow-builder/core/internal/model"
	"github.com/workflow-builder/core/pkg/logger"
	"gorm.io/gorm"
)

// LiveExecution is the concrete implementation of the Execution interface.
// It is built by the worker before invoking a workflow handler.
type LiveExecution struct {
	execID     uuid.UUID
	businessID uuid.UUID
	vars       map[string]string
	ctx        context.Context
	db         *gorm.DB
	rdb        *redis.Client
	encKey     []byte
	log        *executionLogger
	stopTime   string // e.g. "21:00"
}

// NewLiveExecution builds a fully-connected execution context.
func NewLiveExecution(
	ctx context.Context,
	execID, businessID uuid.UUID,
	variables string,
	stopTime string,
	db *gorm.DB,
	rdb *redis.Client,
	encKey []byte,
) *LiveExecution {
	vars := make(map[string]string)
	_ = json.Unmarshal([]byte(variables), &vars)

	le := &LiveExecution{
		execID:     execID,
		businessID: businessID,
		vars:       vars,
		ctx:        ctx,
		db:         db,
		rdb:        rdb,
		encKey:     encKey,
		stopTime:   stopTime,
	}
	le.log = newExecutionLogger(db, execID)
	return le
}

func (e *LiveExecution) ID() uuid.UUID         { return e.execID }
func (e *LiveExecution) BusinessID() uuid.UUID  { return e.businessID }
func (e *LiveExecution) Context() context.Context { return e.ctx }
func (e *LiveExecution) Logger() logger.Logger  { return e.log }

func (e *LiveExecution) GetVar(key string) string {
	return e.vars[key]
}

func (e *LiveExecution) GetIntVar(key string, defaultVal int) int {
	v, ok := e.vars[key]
	if !ok || v == "" {
		return defaultVal
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return defaultVal
	}
	return n
}

func (e *LiveExecution) GetCredential(integration string) (string, error) {
	return GetCredential(e.db, e.businessID, integration, e.encKey)
}

func (e *LiveExecution) GetCredentialByID(id uuid.UUID) (string, error) {
	return GetCredentialByID(e.db, id, e.encKey)
}

func (e *LiveExecution) GetCredentialByVar(key string) (string, error) {
	idStr := e.GetVar(key)
	if idStr == "" {
		return "", fmt.Errorf("variable %s is empty", key)
	}
	id, err := uuid.Parse(idStr)
	if err != nil {
		return "", fmt.Errorf("variable %s contains invalid UUID: %v", key, err)
	}
	return e.GetCredentialByID(id)
}

// IsStopped checks Redis for a stop signal set by the UI.
func (e *LiveExecution) IsStopped() bool {
	select {
	case <-e.ctx.Done():
		return true
	default:
		val, err := e.rdb.Get(e.ctx, "stop:"+e.execID.String()).Result()
		if err != nil {
			return false
		}
		return val == "1"
	}
}

func (e *LiveExecution) ShouldStop() bool {
	if e.IsStopped() {
		return true
	}

	if e.stopTime == "" {
		return false
	}

	// Indonesia WIB is Asia/Jakarta
	loc, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		return false // fallback
	}

	now := time.Now().In(loc)
	current := now.Format("15:04")

	// Simple string comparison for HH:MM
	// Works for checking if we are PAST the stop time on the same day
	if current >= e.stopTime {
		e.log.Info("📍 Automatic stop time reached (%s WIB). Shutting down...", e.stopTime)
		return true
	}

	return false
}

// ReconstructExecution rebuilds an execution context from its ID (for webhook callbacks).
func ReconstructExecution(ctx context.Context, executionID string, db *gorm.DB, rdb *redis.Client, encKey []byte) (Execution, error) {
	execUUID, err := uuid.Parse(executionID)
	if err != nil {
		return nil, fmt.Errorf("invalid execution id: %v", err)
	}

	var exec model.Execution
	if err := db.Preload("Workflow").First(&exec, "id = ?", execUUID).Error; err != nil {
		return nil, fmt.Errorf("execution not found: %v", err)
	}

	return NewLiveExecution(ctx, execUUID, exec.Workflow.BusinessID, exec.Workflow.Variables, exec.Workflow.StopTime, db, rdb, encKey), nil
}

// --- Execution Logger: writes to both stdout AND execution_logs table ---

type executionLogger struct {
	db     *gorm.DB
	execID uuid.UUID
	std    logger.Logger
}

func newExecutionLogger(db *gorm.DB, execID uuid.UUID) *executionLogger {
	return &executionLogger{db: db, execID: execID, std: logger.New()}
}

func (l *executionLogger) writeLog(level, msg string) {
	l.db.Create(&model.ExecutionLog{
		ExecutionID: l.execID,
		Level:       level,
		Message:     msg,
		CreatedAt:   time.Now(),
	})
}

func (l *executionLogger) Info(msg string, args ...interface{})  { m := fmt.Sprintf(msg, args...); l.std.Info(m); l.writeLog("INFO", m) }
func (l *executionLogger) Infof(f string, args ...interface{})   { m := fmt.Sprintf(f, args...); l.std.Info(m); l.writeLog("INFO", m) }
func (l *executionLogger) Warn(msg string, args ...interface{})  { m := fmt.Sprintf(msg, args...); l.std.Warn(m); l.writeLog("WARN", m) }
func (l *executionLogger) Warnf(f string, args ...interface{})   { m := fmt.Sprintf(f, args...); l.std.Warn(m); l.writeLog("WARN", m) }
func (l *executionLogger) Error(msg string, args ...interface{}) { m := fmt.Sprintf(msg, args...); l.std.Error(m); l.writeLog("ERROR", m) }
func (l *executionLogger) Errorf(f string, args ...interface{})  { m := fmt.Sprintf(f, args...); l.std.Error(m); l.writeLog("ERROR", m) }
