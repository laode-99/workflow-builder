package leadflow

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/workflow-builder/core/internal/integrations/openai"
	"github.com/workflow-builder/core/internal/model"
	"github.com/workflow-builder/core/internal/repo"
	"github.com/workflow-builder/core/internal/sdk"
	"github.com/workflow-builder/core/internal/statemachine"
)

// --- Task constructors ---

// newIntentClassifyTask builds an Asynq task to classify the chatbot-side
// intent of a lead after one of their user turns.
func newIntentClassifyTask(businessID, leadID uuid.UUID) (*asynq.Task, error) {
	body, err := json.Marshal(LeadTaskPayload{
		BusinessID: businessID,
		LeadID:     leadID,
	})
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TaskChatbotClassifyIntent, body,
		asynq.Queue(QueueDefault),
		asynq.MaxRetry(2),
	), nil
}

// newSpamClassifyTask builds an Asynq task to run the spam classifier
// after a lead crosses the configured message count threshold.
func newSpamClassifyTask(businessID, leadID uuid.UUID) (*asynq.Task, error) {
	body, err := json.Marshal(LeadTaskPayload{
		BusinessID: businessID,
		LeadID:     leadID,
	})
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TaskChatbotClassifySpam, body,
		asynq.Queue(QueueDefault),
		asynq.MaxRetry(2),
	), nil
}

// --- Task handlers ---

// handleIntentClassifyTask runs the intent classifier LLM call and applies
// the resulting patch + audit. On Tidak Tertarik / Agent results the lead
// picks up the corresponding terminal flag and fires a CRM sync intent.
func handleIntentClassifyTask(ctx context.Context, t *asynq.Task) error {
	var p LeadTaskPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("unmarshal intent payload: %w", err)
	}

	leadRepo := repo.NewLeadRepo(deps.DB)
	lead, err := leadRepo.GetByID(ctx, p.LeadID)
	if err != nil {
		return fmt.Errorf("load lead: %w", err)
	}
	// Don't re-classify terminal leads.
	if lead.TerminalSpam || lead.TerminalNotInterested || lead.TerminalAgent {
		return nil
	}

	msgRepo := repo.NewMessageRepo(deps.DB)
	msgs, err := msgRepo.ListWindow(ctx, lead.ID, 8)
	if err != nil {
		return err
	}
	if len(msgs) == 0 {
		return nil
	}

	promptRepo := repo.NewPromptRepo(deps.DB)
	prompt, err := promptRepo.GetActive(ctx, p.BusinessID, "intent_classifier")
	if err != nil {
		deps.Log.Warnf("no active intent_classifier prompt: %v", err)
		return nil
	}

	oaKey, err := sdk.GetCredential(deps.DB, p.BusinessID, "openai", deps.EncKey)
	if err != nil {
		return err
	}
	oa := openai.NewClient(strings.TrimSpace(oaKey))

	var biz model.Business
	_ = deps.DB.WithContext(ctx).First(&biz, "id = ?", p.BusinessID).Error
	var cfg statemachine.FlowConfig
	cfg, _ = statemachine.LoadConfig([]byte(biz.Config))
	classifierModel := cfg.Chatbot.IntentClassifierModel
	if classifierModel == "" {
		classifierModel = "gpt-4o-mini"
	}

	resp, err := oa.ChatCompletion(ctx, openai.ChatRequest{
		Model:       classifierModel,
		Temperature: 0,
		Messages: []openai.ChatMessage{
			{Role: "system", Content: prompt.Content},
			{Role: "user", Content: formatClassifierHistory(msgs) + "\n\nRespond with EXACTLY one of: Callback | Tidak Tertarik | Agent"},
		},
	})
	if err != nil {
		return fmt.Errorf("intent classifier llm: %w", err)
	}
	raw, _ := openai.ExtractText(resp)
	intent := normalizeIntentResult(raw)

	deps.Log.Infof("intent classified: lead=%s intent=%q", lead.ID, intent)

	// Run the state machine to get the patch + commands for this intent.
	patch, cmds, err := statemachine.Transition(
		statemachine.LeadToStatemachine(*lead),
		statemachine.EventIntentClassified{Intent: intent},
		cfg,
		time.Now(),
		true,
	)
	if err != nil {
		return err
	}
	if patch.IsEmpty() && len(cmds) == 0 {
		return nil
	}
	updated, err := leadRepo.Transition(ctx, lead.ID, lead.Version, patch, repo.AuditEntry{
		Actor:     "intent_classifier",
		EventType: "intent_classified",
		Reason:    intent,
	})
	if err != nil {
		deps.Log.Warnf("intent classifier transition: %v", err)
		return nil
	}
	EnqueueCommands(ctx, p.BusinessID, lead.ID, updated.Version, cmds)
	return nil
}

// handleSpamClassifyTask runs the binary spam classifier. Only fires on
// leads whose chat_total >= config.spam_classify_threshold.
func handleSpamClassifyTask(ctx context.Context, t *asynq.Task) error {
	var p LeadTaskPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("unmarshal spam payload: %w", err)
	}

	leadRepo := repo.NewLeadRepo(deps.DB)
	lead, err := leadRepo.GetByID(ctx, p.LeadID)
	if err != nil {
		return err
	}
	if lead.TerminalSpam {
		return nil
	}

	msgRepo := repo.NewMessageRepo(deps.DB)
	msgs, err := msgRepo.ListWindow(ctx, lead.ID, 10)
	if err != nil {
		return err
	}

	promptRepo := repo.NewPromptRepo(deps.DB)
	prompt, err := promptRepo.GetActive(ctx, p.BusinessID, "spam_classifier")
	if err != nil {
		return nil
	}

	oaKey, err := sdk.GetCredential(deps.DB, p.BusinessID, "openai", deps.EncKey)
	if err != nil {
		return err
	}
	oa := openai.NewClient(strings.TrimSpace(oaKey))

	var biz model.Business
	_ = deps.DB.WithContext(ctx).First(&biz, "id = ?", p.BusinessID).Error
	var cfg statemachine.FlowConfig
	cfg, _ = statemachine.LoadConfig([]byte(biz.Config))
	classifierModel := cfg.Chatbot.SpamClassifierModel
	if classifierModel == "" {
		classifierModel = "gpt-4o-mini"
	}

	resp, err := oa.ChatCompletion(ctx, openai.ChatRequest{
		Model:       classifierModel,
		Temperature: 0,
		Messages: []openai.ChatMessage{
			{Role: "system", Content: prompt.Content},
			{Role: "user", Content: formatClassifierHistory(msgs) + "\n\nRespond with EXACTLY one word: spam OR not_spam"},
		},
	})
	if err != nil {
		return err
	}
	raw, _ := openai.ExtractText(resp)
	lower := strings.ToLower(strings.TrimSpace(raw))
	isSpam := strings.Contains(lower, "spam") && !strings.Contains(lower, "not_spam") && !strings.Contains(lower, "not spam")
	if !isSpam {
		return nil
	}

	// Flip chatbot_states.spam_flag so future chatbot turns are skipped.
	stateRepo := repo.NewChatbotStateRepo(deps.DB)
	_ = stateRepo.SetSpam(ctx, lead.ID)

	patch, cmds, _ := statemachine.Transition(
		statemachine.LeadToStatemachine(*lead),
		statemachine.EventSpamClassified{IsSpam: true},
		cfg,
		time.Now(),
		true,
	)
	updated, err := leadRepo.Transition(ctx, lead.ID, lead.Version, patch, repo.AuditEntry{
		Actor:     "spam_classifier",
		EventType: "spam_classified",
	})
	if err != nil {
		return nil
	}
	EnqueueCommands(ctx, p.BusinessID, lead.ID, updated.Version, cmds)
	return nil
}

// --- Helpers ---

func formatClassifierHistory(msgs []model.LeadMessage) string {
	var sb strings.Builder
	for _, m := range msgs {
		role := "User"
		if m.Role == "assistant" {
			role = "AI"
		}
		sb.WriteString(fmt.Sprintf("%s: %s\n", role, m.Content))
	}
	return sb.String()
}

// normalizeIntentResult takes the raw LLM output and returns one of the
// three valid intent constants. Unknown output falls back to Callback
// (the safest non-terminal classification).
func normalizeIntentResult(raw string) string {
	s := strings.ToLower(strings.TrimSpace(raw))
	s = strings.Trim(s, "\"'. \n\t")
	switch {
	case strings.Contains(s, "tidak tertarik"):
		return statemachine.IntentTidakTertarik
	case strings.Contains(s, "agent"):
		return statemachine.IntentAgent
	case strings.Contains(s, "callback"):
		return statemachine.IntentCallback
	}
	return statemachine.IntentCallback
}

