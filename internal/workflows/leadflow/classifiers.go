package leadflow

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/workflow-builder/core/internal/integrations/openai"
	"github.com/workflow-builder/core/internal/model"
	"github.com/workflow-builder/core/internal/repo"
	"github.com/workflow-builder/core/internal/sdk"
	"github.com/workflow-builder/core/internal/statemachine"
)

func handleIntentClassify(ctx context.Context, exec sdk.Execution) error {
	log := exec.Logger()
	bizID := exec.BusinessID()
	db := exec.GetDB()

	leadIDStr := exec.GetVar("lead_id")
	leadID, err := uuid.Parse(leadIDStr)
	if err != nil {
		return err
	}

	leadRepo := repo.NewLeadRepo(db)
	lead, err := leadRepo.GetByID(ctx, leadID)
	if err != nil {
		return err
	}

	// 1. Get History (Last 4 user messages as per spec)
	msgRepo := repo.NewMessageRepo(db)
	msgs, err := msgRepo.ListWindow(ctx, leadID, 4)
	if err != nil {
		return err
	}

	history := formatClassifierHistory(msgs)

	// 2. Fetch Prompt
	promptRepo := repo.NewPromptRepo(db)
	prompt, err := promptRepo.GetActive(ctx, bizID, "intent_classifier")
	if err != nil {
		return err
	}

	// 3. Call OpenAI
	oaKey, _ := exec.GetCredential("openai")
	oaClient := openai.NewClient(oaKey)

	resp, err := oaClient.ChatCompletion(ctx, openai.ChatRequest{
		Model: "gpt-4o-mini",
		Messages: []openai.ChatMessage{
			{Role: "system", Content: prompt.Content},
			{Role: "user", Content: history},
		},
		Temperature: 0,
	})
	if err != nil {
		return err
	}

	intent, _ := openai.ExtractText(resp)
	intent = strings.TrimSpace(intent)

	// Intent must be one of: Callback, Tidak Tertarik, Agent
	// Statemachine handles the flags.
	log.Infof("Lead %s classified as: %s", lead.ID, intent)

	var biz model.Business
	if err := db.WithContext(ctx).First(&biz, "id = ?", bizID).Error; err != nil {
		return err
	}
	var flowCfg statemachine.FlowConfig
	_ = json.Unmarshal([]byte(biz.Config), &flowCfg)

	patch, commands, err := statemachine.Transition(
		statemachine.LeadToStatemachine(*lead),
		statemachine.EventIntentClassified{Intent: intent},
		flowCfg,
		time.Now(),
		true,
	)

	if err != nil {
		return err
	}

	audit := repo.AuditEntry{
		Actor:     "intent_classifier",
		EventType: "intent_classified",
		Reason:    intent,
	}
	_, err = leadRepo.TransitionTx(ctx, db, lead.ID, lead.Version, patch, audit)
	if err != nil {
		return err
	}

	// TODO: Enqueue commands (e.g. CRM Sync)
	log.Infof("Intent transition applied. Commands: %d", len(commands))

	return nil
}


func formatClassifierHistory(msgs []model.LeadMessage) string {
	var sb strings.Builder
	for _, m := range msgs {
		role := m.Role
		if role == "assistant" {
			role = "AI"
		} else {
			role = "User"
		}
		sb.WriteString(fmt.Sprintf("%s: %s\n", role, m.Content))
	}
	return sb.String()
}

func handleSpamClassify(ctx context.Context, exec sdk.Execution) error {
	log := exec.Logger()
	bizID := exec.BusinessID()
	db := exec.GetDB()

	leadIDStr := exec.GetVar("lead_id")
	leadID, err := uuid.Parse(leadIDStr)
	if err != nil {
		return err
	}

	leadRepo := repo.NewLeadRepo(db)
	lead, err := leadRepo.GetByID(ctx, leadID)
	if err != nil {
		return err
	}

	// Get last 5 user messages for the spam classifier window.
	msgRepo := repo.NewMessageRepo(db)
	msgs, err := msgRepo.ListWindow(ctx, leadID, 10)
	if err != nil {
		return err
	}

	// Load the spam classifier prompt.
	promptRepo := repo.NewPromptRepo(db)
	prompt, err := promptRepo.GetActive(ctx, bizID, "spam_classifier")
	if err != nil {
		log.Warnf("No active spam_classifier prompt for project %s: %v", bizID, err)
		return nil
	}

	// Call OpenAI.
	oaKey, err := exec.GetCredential("openai")
	if err != nil {
		return err
	}
	oaClient := openai.NewClient(oaKey)

	history := formatClassifierHistory(msgs)
	resp, err := oaClient.ChatCompletion(ctx, openai.ChatRequest{
		Model: "gpt-4o-mini",
		Messages: []openai.ChatMessage{
			{Role: "system", Content: prompt.Content},
			{Role: "user", Content: history + "\n\nRespond with EXACTLY one word: spam OR not_spam"},
		},
		Temperature: 0,
	})
	if err != nil {
		return err
	}

	rawClass, _ := openai.ExtractText(resp)
	rawClassLower := strings.ToLower(strings.TrimSpace(rawClass))
	isSpam := strings.Contains(rawClassLower, "spam") && !strings.Contains(rawClassLower, "not_spam") && !strings.Contains(rawClassLower, "not spam")

	log.Infof("Spam classification for lead %s: %q (spam=%v)", leadID, rawClass, isSpam)

	if !isSpam {
		return nil
	}

	// Apply transition.
	var biz model.Business
	if err := db.WithContext(ctx).First(&biz, "id = ?", bizID).Error; err != nil {
		return err
	}
	var flowCfg statemachine.FlowConfig
	_ = json.Unmarshal([]byte(biz.Config), &flowCfg)

	patch, commands, err := statemachine.Transition(
		statemachine.LeadToStatemachine(*lead),
		statemachine.EventSpamClassified{IsSpam: true},
		flowCfg,
		time.Now(),
		true,
	)
	if err != nil {
		return err
	}

	// Also flip chatbot_states.spam_flag so the chatbot turn handler ignores
	// this lead immediately.
	stateRepo := repo.NewChatbotStateRepo(db)
	_ = stateRepo.SetSpam(ctx, leadID)

	audit := repo.AuditEntry{
		Actor:     "spam_classifier",
		EventType: "spam_classified",
		Reason:    "llm_classified_spam",
	}
	updated, err := leadRepo.TransitionTx(ctx, db, lead.ID, lead.Version, patch, audit)
	if err != nil {
		return err
	}

	// Enqueue any CRM sync commands.
	EnqueueCommands(ctx, bizID, lead.ID, updated.Version, commands)
	return nil
}
