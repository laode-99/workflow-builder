package leadflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/workflow-builder/core/internal/integrations"
	"github.com/workflow-builder/core/internal/integrations/gupshup"
	"github.com/workflow-builder/core/internal/integrations/retell"
	"github.com/workflow-builder/core/internal/model"
	"github.com/workflow-builder/core/internal/repo"
	"github.com/workflow-builder/core/internal/sdk"
	"github.com/workflow-builder/core/internal/statemachine"
)

// ---- Task type names ----
//
// Per-lead tasks run as raw Asynq tasks (no workflow row, no sdk.Execution).
// Handlers fetch the lead, load per-project credentials directly via
// sdk.GetCredential, call the integration, and apply state updates via
// LeadRepo.Transition.

const (
	TaskRetellDispatchCall  = "leadflow.retell.dispatch_call"
	TaskGupshupSendBridging = "leadflow.gupshup.send_bridging"
	TaskGupshupSendFinal    = "leadflow.gupshup.send_final"
	TaskChatbotProcessTurn  = "leadflow.chatbot.process_turn"
	TaskChatbotClassifyIntent = "leadflow.chatbot.classify_intent"
	TaskChatbotClassifySpam   = "leadflow.chatbot.classify_spam"
	TaskCRMSync               = "leadflow.crm.sync"
)

// Queue priorities.
const (
	QueueCritical = "critical"
	QueueDefault  = "default"
)

// ---- Task payloads ----

// LeadTaskPayload is the common shape: which project, which lead, and the
// lead's expected version at enqueue time (for optimistic-lock guards).
type LeadTaskPayload struct {
	BusinessID      uuid.UUID `json:"business_id"`
	LeadID          uuid.UUID `json:"lead_id"`
	ExpectedVersion int       `json:"expected_version"`
}

// RetellDispatchPayload extends LeadTaskPayload with the attempt level.
type RetellDispatchPayload struct {
	LeadTaskPayload
	Attempt int `json:"attempt"`
}

// ChatbotTurnTaskPayload identifies the inbound message that triggered
// a chatbot turn. The webhook handler enqueues this after inserting the
// message row and applying the state machine transition.
type ChatbotTurnTaskPayload struct {
	LeadTaskPayload
	MessageID uint64 `json:"message_id"`
}

// ---- Task constructors (used by webhook handlers and cron workers) ----

// NewRetellDispatchTask builds an Asynq task for dispatching a Retell call.
func NewRetellDispatchTask(p RetellDispatchPayload) (*asynq.Task, error) {
	body, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TaskRetellDispatchCall, body,
		asynq.Queue(QueueCritical),
		asynq.MaxRetry(3),
		asynq.Timeout(30*time.Second),
	), nil
}

// NewGupshupBridgingTask builds a task for sending the WA bridging message.
func NewGupshupBridgingTask(p LeadTaskPayload) (*asynq.Task, error) {
	body, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TaskGupshupSendBridging, body,
		asynq.Queue(QueueCritical),
		asynq.MaxRetry(3),
	), nil
}

// NewGupshupFinalTask builds a task for sending the final WA message.
func NewGupshupFinalTask(p LeadTaskPayload) (*asynq.Task, error) {
	body, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TaskGupshupSendFinal, body,
		asynq.Queue(QueueCritical),
		asynq.MaxRetry(3),
	), nil
}

// NewChatbotTurnTask builds a task for processing a chatbot turn.
func NewChatbotTurnTask(p ChatbotTurnTaskPayload) (*asynq.Task, error) {
	body, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TaskChatbotProcessTurn, body,
		asynq.Queue(QueueCritical),
		asynq.MaxRetry(2),
	), nil
}

// ---- Task handlers ----

// RegisterTaskHandlers registers all per-lead Asynq task handlers onto the
// provided ServeMux. SetDependencies MUST be called before this.
func RegisterTaskHandlers(mux *asynq.ServeMux) error {
	if deps == nil {
		return errors.New("leadflow: SetDependencies must be called before RegisterTaskHandlers")
	}
	mux.HandleFunc(TaskRetellDispatchCall, handleRetellDispatchTask)
	mux.HandleFunc(TaskGupshupSendBridging, handleGupshupBridgingTask)
	mux.HandleFunc(TaskGupshupSendFinal, handleGupshupFinalTask)
	mux.HandleFunc(TaskChatbotProcessTurn, handleChatbotTurnTask)
	mux.HandleFunc(TaskChatbotClassifyIntent, handleIntentClassifyTask)
	mux.HandleFunc(TaskChatbotClassifySpam, handleSpamClassifyTask)
	return nil
}

// handleRetellDispatchTask loads the lead, checks it's still eligible
// (version + terminal flags), loads per-project Retell credentials, and
// kicks off the call via Retell API.
func handleRetellDispatchTask(ctx context.Context, t *asynq.Task) error {
	var p RetellDispatchPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("unmarshal retell dispatch: %w", err)
	}

	leadRepo := repo.NewLeadRepo(deps.DB)
	lead, err := leadRepo.GetByID(ctx, p.LeadID)
	if err != nil {
		return fmt.Errorf("load lead: %w", err)
	}

	// Guards: skip if the lead has moved past the expected version or hit
	// a terminal state. Return nil so Asynq does not retry.
	if p.ExpectedVersion > 0 && lead.Version != p.ExpectedVersion {
		deps.Log.Infof("retell dispatch skipped (version moved): lead=%s", lead.ID)
		return nil
	}
	if isTerminal(lead) || lead.WhatsappReplyAt != nil {
		deps.Log.Infof("retell dispatch skipped (terminal): lead=%s", lead.ID)
		return nil
	}

	// Load Retell credentials for this project.
	credRaw, err := sdk.GetCredential(deps.DB, lead.BusinessID, "retell_ai", deps.EncKey)
	if err != nil {
		return fmt.Errorf("load retell creds: %w", err)
	}
	var creds struct {
		APIKey     string `json:"api_key"`
		AgentID1   string `json:"agent_id_1"`
		AgentID3   string `json:"agent_id_3"`
		FromNumber string `json:"from_number"`
	}
	if err := json.Unmarshal([]byte(credRaw), &creds); err != nil {
		return fmt.Errorf("parse retell creds: %w", err)
	}

	agentID := creds.AgentID1
	if p.Attempt >= 3 && creds.AgentID3 != "" {
		agentID = creds.AgentID3
	}

	client := retell.NewClient(creds.APIKey)
	dynamicVars := map[string]interface{}{
		"first_name": lead.Name,
		"leads_id":   lead.ExternalID,
		"attempt":    fmt.Sprintf("%d", p.Attempt),
	}
	metadata := map[string]string{
		"project_id": lead.BusinessID.String(),
		"lead_id":    lead.ID.String(),
		"attempt":    fmt.Sprintf("%d", p.Attempt),
	}

	call, err := client.CreateCall(ctx, agentID, lead.Phone, creds.FromNumber, dynamicVars, metadata)
	if err != nil {
		return classifyAsynqError(err)
	}
	deps.Log.Infof("retell call dispatched: lead=%s call_id=%s attempt=%d", lead.ID, call.ID, p.Attempt)
	return nil
}

// handleGupshupBridgingTask sends the WA bridging message after attempt 1.
func handleGupshupBridgingTask(ctx context.Context, t *asynq.Task) error {
	return sendGupshupTemplate(ctx, t, "bridging")
}

// handleGupshupFinalTask sends the WA final message at attempt 5.
func handleGupshupFinalTask(ctx context.Context, t *asynq.Task) error {
	return sendGupshupTemplate(ctx, t, "final")
}

func sendGupshupTemplate(ctx context.Context, t *asynq.Task, kind string) error {
	var p LeadTaskPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("unmarshal gupshup %s: %w", kind, err)
	}

	leadRepo := repo.NewLeadRepo(deps.DB)
	lead, err := leadRepo.GetByID(ctx, p.LeadID)
	if err != nil {
		return fmt.Errorf("load lead: %w", err)
	}
	if isTerminal(lead) {
		return nil
	}

	// Load Gupshup credentials.
	credRaw, err := sdk.GetCredential(deps.DB, lead.BusinessID, "gupshup", deps.EncKey)
	if err != nil {
		return fmt.Errorf("load gupshup creds: %w", err)
	}
	var creds gupshup.Credentials
	if err := json.Unmarshal([]byte(credRaw), &creds); err != nil {
		return fmt.Errorf("parse gupshup creds: %w", err)
	}

	// Load the appropriate message template from project_prompts.
	promptRepo := repo.NewPromptRepo(deps.DB)
	promptKind := "wa_bridging"
	if kind == "final" {
		promptKind = "wa_final"
	}
	msgPrompt, err := promptRepo.GetActive(ctx, lead.BusinessID, promptKind)
	if err != nil {
		return fmt.Errorf("load %s template: %w", promptKind, err)
	}

	client := gupshup.NewClient(creds)
	if err := client.SendText(ctx, lead.Phone, msgPrompt.Content); err != nil {
		return classifyAsynqError(err)
	}
	deps.Log.Infof("gupshup %s sent: lead=%s", kind, lead.ID)

	// Record outbound message for audit/UI.
	msgRepo := repo.NewMessageRepo(deps.DB)
	_ = msgRepo.Insert(ctx, &model.LeadMessage{
		BusinessID: lead.BusinessID,
		LeadID:     lead.ID,
		Direction:  "outbound",
		Role:       "assistant",
		Content:    msgPrompt.Content,
		CreatedAt:  time.Now(),
	})
	return nil
}

// isTerminal returns true if any terminal flag is set on the lead.
func isTerminal(l *model.Lead) bool {
	return l.TerminalInvalid || l.TerminalResponded || l.TerminalNotInterested ||
		l.TerminalSpam || l.TerminalAgent || l.TerminalCompleted
}

// classifyAsynqError translates integration error categories into Asynq
// retry behavior. Asynq retries whenever a handler returns a non-nil error;
// to abort retries we wrap the error with asynq.SkipRetry.
func classifyAsynqError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, integrations.ErrPermanent) || errors.Is(err, integrations.ErrAuth) {
		// Non-retryable: skip to DLQ.
		return fmt.Errorf("%w: %v", asynq.SkipRetry, err)
	}
	return err
}

// EnqueueCommands translates a list of statemachine.Command into Asynq
// task enqueues. This is the connector between the pure state machine and
// the async execution layer. Called by cron workers (attempt_manager,
// classifiers, etc.) AFTER their DB transaction commits.
//
// For CRMSync commands, instead of enqueuing a task, a CRMSyncIntent row
// is created — the singleton CRM outbox poller will process it.
func EnqueueCommands(ctx context.Context, businessID, leadID uuid.UUID, expectedVersion int, commands []statemachine.Command) {
	if deps == nil || deps.Asynq == nil {
		return
	}
	basePayload := LeadTaskPayload{
		BusinessID:      businessID,
		LeadID:          leadID,
		ExpectedVersion: expectedVersion,
	}
	for _, cmd := range commands {
		switch c := cmd.(type) {
		case statemachine.CmdEnqueueRetellCall:
			task, err := NewRetellDispatchTask(RetellDispatchPayload{
				LeadTaskPayload: basePayload,
				Attempt:         c.Attempt,
			})
			if err == nil {
				_, _ = deps.Asynq.EnqueueContext(ctx, task)
			}
		case statemachine.CmdEnqueueWABridging:
			task, err := NewGupshupBridgingTask(basePayload)
			if err == nil {
				_, _ = deps.Asynq.EnqueueContext(ctx, task)
			}
		case statemachine.CmdEnqueueWAFinal:
			task, err := NewGupshupFinalTask(basePayload)
			if err == nil {
				_, _ = deps.Asynq.EnqueueContext(ctx, task)
			}
		case statemachine.CmdEnqueueCRMSync:
			// CRM sync goes through the outbox (not Asynq) so the intent
			// survives Redis restarts.
			intent := &model.CRMSyncIntent{
				BusinessID: businessID,
				LeadID:     leadID,
				Path:       c.Path,
				Payload:    "{}",
				Status:     "pending",
			}
			_ = deps.DB.WithContext(ctx).Create(intent).Error
		}
	}
}
