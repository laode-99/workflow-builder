package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
	"github.com/workflow-builder/core/internal/model"
	"github.com/workflow-builder/core/internal/repo"
	"github.com/workflow-builder/core/internal/statemachine"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// Diagnostic script to verify the Leadflow AI engine parity.
// It creates a test project and walks a lead through the state machine.
func main() {
	_ = godotenv.Load()

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "host=127.0.0.1 user=workflow password=workflow_password dbname=workflow_engine port=5434 sslmode=disable"
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("failed to connect to db: %v", err)
	}

	ctx := context.Background()
	bizID := uuid.New()
	
	fmt.Println("--- PHASE 1: SETUP TEST PROJECT ---")
	
	// Create Business
	biz := &model.Business{
		ID:     bizID,
		Name:   "Diagnostic Project",
		Slug:   fmt.Sprintf("diagnostic-%s", bizID.String()[:8]),
		Status: "active",
		Config: fmt.Sprintf(`{
			"business_hours": {"timezone": "Asia/Jakarta", "start": "08:00", "end": "22:00", "days": [1,2,3,4,5,6,7]},
			"call_retry_gap_hours": 3,
			"voicemail_shortcut_to_last": true,
			"internal_alert_group": "test-group-id"
		}`),
	}
	if err := db.Create(biz).Error; err != nil {
		log.Fatalf("failed to create biz: %v", err)
	}
	fmt.Printf("Created Test Project: %s\n", bizID)

	leadRepo := repo.NewLeadRepo(db)

	fmt.Println("\n--- PHASE 2: TEST EMPTY NAME GUARD ---")
	
	leadNoName := &model.Lead{
		ID:         uuid.New(),
		BusinessID: bizID,
		Phone:      "628123456789",
		Name:       "", // Empty name
		ExternalID: "OPP-101",
		Attempt:    0,
	}
	db.Create(leadNoName)
	
	// Run Transition for Attempt 0
	runStep(ctx, leadRepo, leadNoName, "Empty Name Check", biz.Config)

	fmt.Println("\n--- PHASE 3: TEST DISPATCH CALL 1 ---")
	
	leadWithName := &model.Lead{
		ID:         uuid.New(),
		BusinessID: bizID,
		Phone:      "628999888777",
		Name:       "Budi Diagnostic",
		ExternalID: "OPP-102",
		Attempt:    0,
	}
	db.Create(leadWithName)
	
	runStep(ctx, leadRepo, leadWithName, "Dispatch Call 1", biz.Config)

	fmt.Println("\n--- PHASE 4: TEST VOICEMAIL SHORTCUT ---")
	
	// Simulate a voicemail outcome for Budi
	// reload Budi
	db.First(leadWithName, "id = ?", leadWithName.ID)
	simulateCallOutcome(ctx, leadRepo, leadWithName, "voicemail_reached", "Voicemail", biz.Config)

	fmt.Println("\n--- PHASE 5: TEST INTENT CLASSIFICATION (AGENT) ---")
	
	leadChat := &model.Lead{
		ID:         uuid.New(),
		BusinessID: bizID,
		Phone:      "628555444333",
		Name:       "Siti Chatbot",
		ExternalID: "OPP-103",
		Attempt:    2, // in WA phase
	}
	db.Create(leadChat)
	
	simulateIntent(ctx, leadRepo, leadChat, "Agent", biz.Config)

	fmt.Println("\n--- DIAGNOSTICS COMPLETE ---")
	fmt.Printf("Cleaning up project %s...\n", bizID)
	// db.Unscoped().Delete(biz) // optional
}

func runStep(ctx context.Context, r *repo.LeadRepo, lead *model.Lead, label, config string) {
	fmt.Printf(">>> Step: %s\n", label)
	
	var cfg statemachine.FlowConfig
	json.Unmarshal([]byte(config), &cfg)
	
	loc, _ := time.LoadLocation("Asia/Jakarta")
	now := time.Now().In(loc)
	
	patch, commands, _ := statemachine.Transition(
		statemachine.LeadToStatemachine(*lead),
		statemachine.EventCronTick{},
		cfg,
		now,
		true,
	)
	
	audit := repo.AuditEntry{Actor: "diagnostic", EventType: "cron_tick", Reason: label}
	updated, err := r.Transition(ctx, lead.ID, lead.Version, patch, commands, audit)
	if err != nil {
		fmt.Printf("ERROR: %v\n", err)
		return
	}
	
	fmt.Printf("Result: Attempt %d -> %d\n", lead.Attempt, updated.Attempt)
	fmt.Printf("Commands Enqueued: %d\n", len(commands))
	for _, c := range commands {
		fmt.Printf("- Command: %T\n", c)
	}
}

func simulateCallOutcome(ctx context.Context, r *repo.LeadRepo, lead *model.Lead, reason, customerType, config string) {
	fmt.Printf(">>> Simulating Call Outcome: %s\n", reason)
	
	var cfg statemachine.FlowConfig
	json.Unmarshal([]byte(config), &cfg)
	
	patch, commands, _ := statemachine.Transition(
		statemachine.LeadToStatemachine(*lead),
		statemachine.EventCallAnalyzed{
			Attempt: lead.Attempt,
			DisconnectedReason: reason,
			CustomerType: customerType,
		},
		cfg,
		time.Now(),
		true,
	)
	
	audit := repo.AuditEntry{Actor: "diagnostic", EventType: "call_outcome", Reason: reason}
	updated, err := r.Transition(ctx, lead.ID, lead.Version, patch, commands, audit)
	if err != nil {
		fmt.Printf("ERROR: %v\n", err)
		return
	}
	
	fmt.Printf("Result: Attempt %d -> %d\n", lead.Attempt, updated.Attempt)
	fmt.Printf("Terminal Flag: %v\n", updated.TerminalCompleted || updated.TerminalResponded)
}

func simulateIntent(ctx context.Context, r *repo.LeadRepo, lead *model.Lead, intent, config string) {
	fmt.Printf(">>> Simulating Intent: %s\n", intent)
	
	var cfg statemachine.FlowConfig
	json.Unmarshal([]byte(config), &cfg)
	
	patch, commands, _ := statemachine.Transition(
		statemachine.LeadToStatemachine(*lead),
		statemachine.EventIntentClassified{Intent: intent},
		cfg,
		time.Now(),
		true,
	)
	
	audit := repo.AuditEntry{Actor: "diagnostic", EventType: "intent_classified", Reason: intent}
	updated, err := r.Transition(ctx, lead.ID, lead.Version, patch, commands, audit)
	if err != nil {
		fmt.Printf("ERROR: %v\n", err)
		return
	}
	
	fmt.Printf("Result: Interest2 = %s\n", updated.Interest2)
	fmt.Printf("Terminal Agent: %v\n", updated.TerminalAgent)
}
