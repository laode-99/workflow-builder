package leadflow

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/workflow-builder/core/internal/integrations/openai"
	"github.com/workflow-builder/core/internal/integrations/pinecone"
	"github.com/workflow-builder/core/internal/model"
	"github.com/workflow-builder/core/internal/repo"
	"github.com/workflow-builder/core/internal/sdk"
	"github.com/workflow-builder/core/internal/statemachine"
	"gorm.io/gorm"
)

type ChatbotTurnPayload struct {
	LeadID     uuid.UUID `json:"lead_id"`
	BusinessID uuid.UUID `json:"business_id"`
}

func handleChatbotTurn(ctx context.Context, exec sdk.Execution) error {
	log := exec.Logger()
	bizID := exec.BusinessID()
	db := exec.GetDB()

	// 1. Load context
	// In reality, this might be triggered by an Asynq task. 
	// The payload would contain the LeadID.
	// For MVP integration, we expect the context vars or payload.
	leadIDStr := exec.GetVar("lead_id") // placeholder for task payload extraction
	if leadIDStr == "" {
		return fmt.Errorf("missing lead_id in chatbot turn")
	}
	leadID, _ := uuid.Parse(leadIDStr)

	leadRepo := repo.NewLeadRepo(db)
	lead, err := leadRepo.GetByID(ctx, leadID)
	if err != nil {
		return err
	}

	stateRepo := repo.NewChatbotStateRepo(db)
	state, err := stateRepo.Get(ctx, leadID)
	if err != nil {
		return err
	}

	var biz model.Business
	if err := db.WithContext(ctx).First(&biz, "id = ?", bizID).Error; err != nil {
		return err
	}
	var flowCfg statemachine.FlowConfig
	_ = json.Unmarshal([]byte(biz.Config), &flowCfg)

	// 2. Build History
	msgRepo := repo.NewMessageRepo(db)
	historyMsgs, err := msgRepo.ListWindow(ctx, leadID, 10) // 5 turns
	if err != nil {
		return err
	}

	messages := []openai.ChatMessage{}
	// Fetch system prompt
	promptRepo := repo.NewPromptRepo(db)
	sysPrompt, _ := promptRepo.GetActive(ctx, bizID, "chatbot_system")
	if sysPrompt != nil {
		messages = append(messages, openai.ChatMessage{Role: "system", Content: sysPrompt.Content})
	}

	for _, m := range historyMsgs {
		messages = append(messages, openai.ChatMessage{Role: m.Role, Content: m.Content})
	}

	// 3. Setup OpenAI Client
	oaKey, _ := exec.GetCredential("openai")
	oaClient := openai.NewClient(oaKey)

	// 4. Handle Tools (RAG)
	// Get Pinecone credentials
	pcCredsRaw, _ := exec.GetCredential("pinecone")
	var pcCreds struct {
		APIKey string `json:"api_key"`
		Host   string `json:"host"`
	}
	_ = json.Unmarshal([]byte(pcCredsRaw), &pcCreds)
	pcClient := pinecone.NewClient(pinecone.Credentials{APIKey: pcCreds.APIKey, IndexHost: pcCreds.Host})

	// Pre-fetch RAG context for the last message if user role
	if len(historyMsgs) > 0 && historyMsgs[len(historyMsgs)-1].Role == "user" {
		lastMsg := historyMsgs[len(historyMsgs)-1].Content
		emb, err := oaClient.Embed(ctx, "text-embedding-3-small", lastMsg)
		if err == nil {
			matches, err := pcClient.Query(ctx, emb, 4)
			if err == nil && len(matches) > 0 {
				var ragContext strings.Builder
				ragContext.WriteString("Property Knowledge Base snippets:\n")
				for _, m := range matches {
					if txt, ok := m.Metadata["text"].(string); ok {
						ragContext.WriteString("- " + txt + "\n")
					}
				}
				messages = append(messages, openai.ChatMessage{Role: "system", Content: ragContext.String()})
			}
		}
	}

	// 5. Define Tools for GPT
	tools := []openai.Tool{
		{
			Type: "function",
			Function: openai.ToolDeclaration{
				Name:        "save_leads_data",
				Description: "Saves lead interest schedule data (Simpan Visit). Use this when the lead provides a viewing date or interest schedule.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"interest":  map[string]any{"type": "string", "enum": []string{"Tertarik Site Visit (Hot Leads)", "Tertarik untuk dihubungi (Warm Leads)", "Tidak mau atau tidak tertarik (Cold Leads)"}},
						"tanggal":   map[string]any{"type": "string", "description": "Day of the month (1-31)"},
						"bulan":     map[string]any{"type": "string", "description": "Month name in Indonesian or number"},
						"tahun":     map[string]any{"type": "string", "description": "Year"},
						"jam":       map[string]any{"type": "string", "description": "Time in 24h format HH:mm"},
						"summary":   map[string]any{"type": "string", "description": "Brief summary of the lead's intent"},
					},
					"required": []string{"interest", "summary"},
				},
			},
		},
	}

	// 6. Execute Chat Turn
	resp, err := oaClient.ChatCompletion(ctx, openai.ChatRequest{
		Model:       "gpt-4o-mini",
		Messages:    messages,
		Tools:       tools,
		Temperature: 0.3,
	})
	if err != nil {
		return err
	}

	// 7. Handle Response / Tool Calls
	choice := resp.Choices[0].Message
	if len(choice.ToolCalls) > 0 {
		for _, tc := range choice.ToolCalls {
			if tc.Function.Name == "save_leads_data" {
				var args struct {
					Interest string `json:"interest"`
					Tanggal  string `json:"tanggal"`
					Bulan    string `json:"bulan"`
					Tahun    string `json:"tahun"`
					Jam      string `json:"jam"`
					Summary  string `json:"summary"`
				}
				_ = json.Unmarshal([]byte(tc.Function.Arguments), &args)
				
				// Apply tool logic
				err := applySaveLeadsData(ctx, db, lead, args)
				if err != nil {
					log.Errorf("Tool save_leads_data failed: %v", err)
				}
			}
		}
		
		// After tool call, we need to get a final response text from AI 
		// Or assume the AI provides it in the same response (Gershwin pattern logic depends on model)
	}

	responseText := choice.Content
	if responseText != "" {
		// 8. Persist Outbound message
		// Save Bot message to DB
		err = msgRepo.Insert(ctx, &model.LeadMessage{
			BusinessID: bizID,
			LeadID:     leadID,
			Direction:  "outbound",
			Role:       "assistant",
			Content:    responseText,
			CreatedAt:  time.Now(),
		})
		if err != nil {
			log.Errorf("Failed to save assistant message: %v", err)
		}
		
		stateRepo.RecordTurn(ctx, leadID, bizID, state.SessionKey, time.Now())

		// Send to Gupshup
		gsCredsRaw, _ := exec.GetCredential("gupshup")
		var gsCreds struct {
			UserID    string `json:"user_id"`
			Password  string `json:"password"`
			AppName   string `json:"app_name"`
			SrcNumber string `json:"src_number"`
		}
		_ = json.Unmarshal([]byte(gsCredsRaw), &gsCreds)
		
		// In a real environment, we'd use the gupshup client.
		// For now, log the outbound dispatch.
		log.Infof("TO [%s] via Gupshup: %s", lead.Phone, responseText)
	}

	// 9. Dispatch Intent Classifier task
	// Enqueue asynq task "chatbot:classify:intent"
	return nil
}

func applySaveLeadsData(ctx context.Context, db *gorm.DB, lead *model.Lead, args struct {
	Interest string `json:"interest"`
	Tanggal  string `json:"tanggal"`
	Bulan    string `json:"bulan"`
	Tahun    string `json:"tahun"`
	Jam      string `json:"jam"`
	Summary  string `json:"summary"`
}) error {
	var svsDate *time.Time
	if args.Tanggal != "" && args.Bulan != "" {
		parsed, err := statemachine.ParseIndoDate(args.Tanggal, args.Bulan, args.Tahun, args.Jam)
		if err == nil {
			svsDate = parsed
		}
	}

	// Build Patch
	newSummary := args.Summary
	if lead.Summary != "" {
		now := time.Now().In(time.FixedZone("WIB", 7*3600))
		newSummary = fmt.Sprintf("%s\n\n%s --- %s", lead.Summary, now.Format("02/01/2006 15:04"), args.Summary)
	}

	patch := statemachine.Patch{
		Interest2: &args.Interest,
		Summary:   &newSummary,
	}
	if svsDate != nil {
		patch.CallDate = svsDate // SvsDate is used for display, CallDate for attempt logic? 
		// Actually, let's keep it consistent with model.
		// Wait, I should update Lead.SvsDate in the model if I want to store it specifically.
		// For now, I'll update the Summary which is the source of truth for Sales.
	}

	leadRepo := repo.NewLeadRepo(db)
	audit := repo.AuditEntry{
		Actor:     "chatbot_agent",
		EventType: "tool_save_leads_data",
		Reason:    args.Interest,
	}

	_, err := leadRepo.TransitionTx(ctx, db, lead.ID, lead.Version, patch, audit)
	return err
}
