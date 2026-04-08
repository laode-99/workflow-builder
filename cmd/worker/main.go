package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
	"github.com/workflow-builder/core/internal/model"
	"github.com/workflow-builder/core/internal/sdk"
	"github.com/workflow-builder/core/pkg/logger"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	_ "github.com/workflow-builder/core/internal/workflows/mortgage"
	_ "github.com/workflow-builder/core/internal/workflows/n8n"
)

type taskPayload struct {
	WorkflowID  string `json:"workflow_id"`
	ExecutionID string `json:"execution_id"`
}

func main() {
	_ = godotenv.Load() // Load .env file if it exists
	l := logger.New()

	// --- Config ---
	redisAddr := getEnv("REDIS_ADDR", "127.0.0.1:6379")
	dsn := getEnv("DATABASE_URL", "host=127.0.0.1 user=workflow password=workflow_password dbname=workflow_engine port=5434 sslmode=disable")
	encKey := []byte(getEnv("ENCRYPTION_KEY", "01234567890123456789012345678901")) // 32 bytes

	// --- Database ---
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("Worker DB connection failed: %v", err)
	}
	l.Info("Worker connected to database")

	// --- Redis & Asynq Worker ---
	var rdb *redis.Client
	var redisOpt asynq.RedisClientOpt

	redisURL := os.Getenv("REDIS_URL")
	if redisURL != "" {
		opt, _ := redis.ParseURL(redisURL)
		rdb = redis.NewClient(opt)
		redisOpt = asynq.RedisClientOpt{
			Addr:     opt.Addr,
			Password: opt.Password,
			DB:       opt.DB,
		}
	} else {
		rdb = redis.NewClient(&redis.Options{Addr: redisAddr})
		redisOpt = asynq.RedisClientOpt{Addr: redisAddr}
	}

	srv := asynq.NewServer(
		redisOpt,
		asynq.Config{
			Concurrency: 10,
			Queues: map[string]int{
				"critical":   6,
				"executions": 3,
				"default":    1,
			},
		},
	)

	mux := asynq.NewServeMux()

	// The main task handler: dispatches to SDK registry
	mux.HandleFunc("workflow:execute", func(ctx context.Context, t *asynq.Task) error {
		var p taskPayload
		if err := json.Unmarshal(t.Payload(), &p); err != nil {
			l.Errorf("Invalid task payload: %v", err)
			return err
		}

		wfID, _ := uuid.Parse(p.WorkflowID)
		execID, _ := uuid.Parse(p.ExecutionID)

		// 1. Load workflow from DB
		var wf model.Workflow
		if err := db.First(&wf, "id = ?", wfID).Error; err != nil {
			l.Errorf("Workflow %s not found: %v", p.WorkflowID, err)
			updateExecution(db, execID, "failed", err.Error())
			return err
		}

		// 2. Lookup handler in SDK registry
		def, ok := sdk.Registry.Workflows[wf.Signature]
		if !ok {
			l.Errorf("Signature '%s' not found in SDK registry", wf.Signature)
			updateExecution(db, execID, "failed", "workflow signature not in registry")
			return nil
		}

		// 3. Build live execution context
		exec := sdk.NewLiveExecution(ctx, execID, wf.BusinessID, wf.Variables, wf.StopTime, db, rdb, encKey)

		// 4. Mark as running
		now := time.Now()
		db.Model(&model.Execution{}).Where("id = ?", execID).Updates(map[string]interface{}{
			"status":     "running",
			"started_at": now,
		})

		l.Infof("▶ Executing %s (exec: %s)", wf.Alias, execID.String()[:8])

		// 5. Call the handler
		handlerErr := def.Handler(ctx, exec)

		// 6. Update execution result
		if handlerErr != nil {
			l.Errorf("✗ Workflow failed: %v", handlerErr)
			updateExecution(db, execID, "failed", handlerErr.Error())
		} else {
			l.Infof("✓ Workflow completed: %s", wf.Alias)
			updateExecution(db, execID, "completed", "")
		}

		return nil // always return nil so asynq doesn't retry
	})

	l.Info("Starting Workflow Worker connected to " + redisAddr)
	if err := srv.Start(mux); err != nil {
		log.Fatalf("Could not start Asynq server: %v", err)
	}

	// Wait for shutdown signal
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGTERM, syscall.SIGINT)
	<-sigs

	l.Info("Shutting down worker...")
	srv.Stop()
	l.Info("Worker stopped.")
}

func updateExecution(db *gorm.DB, execID uuid.UUID, status, errMsg string) {
	now := time.Now()
	db.Model(&model.Execution{}).Where("id = ?", execID).Updates(map[string]interface{}{
		"status":       status,
		"error_msg":    errMsg,
		"completed_at": now,
	})
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
