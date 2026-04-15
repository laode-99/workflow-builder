package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	fiberlog "github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/hibiken/asynq"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
	"github.com/workflow-builder/core/internal/api"
	"github.com/workflow-builder/core/internal/api/handlers/webhooks"
	"github.com/workflow-builder/core/internal/model"
	pkglog "github.com/workflow-builder/core/pkg/logger"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	_ "github.com/workflow-builder/core/internal/workflows/mortgage"
	_ "github.com/workflow-builder/core/internal/workflows/n8n"
)

func main() {
	_ = godotenv.Load() // Load .env file if it exists
	l := pkglog.New()

	// --- Config ---
	dsn := getEnv("DATABASE_URL", "host=127.0.0.1 user=workflow password=workflow_password dbname=workflow_engine port=5434 sslmode=disable")
	redisAddr := getEnv("REDIS_ADDR", "127.0.0.1:6379")
	encKey := []byte(getEnv("ENCRYPTION_KEY", "01234567890123456789012345678901")) // 32 bytes

	// --- Database ---
	var db *gorm.DB
	var err error
	for i := 0; i < 5; i++ {
		db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
		if err == nil {
			break
		}
		l.Infof("DB attempt %d failed, retrying...", i+1)
		time.Sleep(2 * time.Second)
	}
	if err != nil {
		log.Fatalf("DB connection failed: %v", err)
	}
	l.Info("Connected to database")

	if err := db.AutoMigrate(
		&model.Business{},
		&model.Credential{},
		&model.Workflow{},
		&model.Execution{},
		&model.ExecutionLog{},
		// Leadflow engine entities
		&model.Lead{},
		&model.LeadMessage{},
		&model.LeadAudit{},
		&model.LeadAuditArchive{},
		&model.CallEvent{},
		&model.ChatbotState{},
		&model.SalesAssignment{},
		&model.LeadSalesAssignment{},
		&model.ProjectPrompt{},
		&model.CRMSyncIntent{},
	); err != nil {
		log.Fatalf("AutoMigrate failed: %v", err)
	}
	l.Info("Database migrations applied")

	// --- Redis + Asynq ---
	var rdb *redis.Client
	var asynqClient *asynq.Client

	redisURL := os.Getenv("REDIS_URL")
	if redisURL != "" {
		opt, _ := redis.ParseURL(redisURL)
		rdb = redis.NewClient(opt)
		asynqClient = asynq.NewClient(asynq.RedisClientOpt{
			Addr:     opt.Addr,
			Password: opt.Password,
			DB:       opt.DB,
		})
	} else {
		rdb = redis.NewClient(&redis.Options{Addr: redisAddr})
		asynqClient = asynq.NewClient(asynq.RedisClientOpt{Addr: redisAddr})
	}
	defer asynqClient.Close()

	// --- Fiber App ---
	app := fiber.New(fiber.Config{
		AppName: "FlowBuilder API",
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		},
	})

	app.Use(fiberlog.New(fiberlog.Config{
		Format:     "${time} | ${status} | ${latency} | ${method} ${path}\n",
		TimeFormat: "15:04:05",
	}))
	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowMethods: "GET,POST,PATCH,DELETE,OPTIONS",
		AllowHeaders: "Content-Type",
	}))

	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok"})
	})

	// API Routes
	repo := api.NewRepo(db)
	handler := api.NewHandler(repo, asynqClient, rdb, encKey)
	handler.RegisterRoutes(app)

	// Leadflow webhook routes (Retell + chat inbound forwarder).
	// Per-project HMAC verification happens inside each handler.
	retellHandler := webhooks.NewRetellHandler(db, encKey, l)
	chatInboundHandler := webhooks.NewChatInboundHandler(db, encKey, l)
	app.Post("/api/webhooks/retell/:slug", retellHandler.Handle)
	app.Post("/api/webhooks/chat-inbound/:slug", chatInboundHandler.Handle)

	// --- Start Background Scheduler ---
	scheduler := api.NewScheduler(repo, asynqClient)
	scheduler.Start(context.Background())

	port := getEnv("PORT", "8080")
	l.Infof("FlowBuilder API starting on :%s", port)
	if err := app.Listen(":" + port); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
