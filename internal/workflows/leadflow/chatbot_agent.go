package leadflow

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/workflow-builder/core/internal/integrations/gupshup"
	"github.com/workflow-builder/core/internal/integrations/openai"
	"github.com/workflow-builder/core/internal/integrations/pinecone"
	"github.com/workflow-builder/core/internal/model"
	"github.com/workflow-builder/core/internal/repo"
	"github.com/workflow-builder/core/internal/sdk"
	"github.com/workflow-builder/core/internal/statemachine"
)

// handleChatbotTurnTask is the Asynq handler for TaskChatbotProcessTurn.
// Triggered by the chat-inbound webhook after a new user message lands.
//
// Flow:
//  1. Load lead + chatbot state + business
//  2. Early-exit if the lead is flagged as spam (chatbot blocked)
//  3. Build the OpenAI messages (system + FAQ + tool instructions + history)
//  4. Run the tool-call loop up to config.max_tool_iterations
//     - property_knowledge tool: Pinecone RAG
//     - save_leads_data tool: upsert lead fields locally
//  5. Send the final text reply via Gupshup
//  6. Persist the assistant message + update chatbot_states
//  7. Enqueue intent + optional spam classifier tasks
func handleChatbotTurnTask(ctx context.Context, t *asynq.Task) error {
	var p ChatbotTurnTaskPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("unmarshal chatbot turn payload: %w", err)
	}

	// --- 1. Load lead + state + business config ---
	leadRepo := repo.NewLeadRepo(deps.DB)
	lead, err := leadRepo.GetByID(ctx, p.LeadID)
	if err != nil {
		return fmt.Errorf("load lead: %w", err)
	}

	// Respect chat-blocking terminal flags.
	if lead.TerminalSpam || lead.TerminalNotInterested || lead.TerminalAgent {
		deps.Log.Infof("chatbot turn skipped (terminal): lead=%s", lead.ID)
		return nil
	}

	var biz model.Business
	if err := deps.DB.WithContext(ctx).First(&biz, "id = ?", p.BusinessID).Error; err != nil {
		return fmt.Errorf("load business: %w", err)
	}
	var cfg statemachine.FlowConfig
	cfg, _ = statemachine.LoadConfig([]byte(biz.Config))

	stateRepo := repo.NewChatbotStateRepo(deps.DB)
	state, err := stateRepo.Get(ctx, lead.ID)
	if err != nil && err != repo.ErrNotFound {
		return fmt.Errorf("load chatbot state: %w", err)
	}
	if state != nil && state.SpamFlag {
		deps.Log.Infof("chatbot turn skipped (spam flag): lead=%s", lead.ID)
		return nil
	}

	// --- 2. Load conversation window ---
	msgRepo := repo.NewMessageRepo(deps.DB)
	windowSize := cfg.Chatbot.WindowSize
	if windowSize == 0 {
		windowSize = 4
	}
	history, err := msgRepo.ListWindow(ctx, lead.ID, windowSize*2)
	if err != nil {
		return fmt.Errorf("load message window: %w", err)
	}

	// --- 3. Load prompts (system + FAQ + tool instructions) ---
	promptRepo := repo.NewPromptRepo(deps.DB)
	sysPrompt, _ := promptRepo.GetActive(ctx, p.BusinessID, "chatbot_system")
	faqPrompt, _ := promptRepo.GetActive(ctx, p.BusinessID, "chatbot_faq")
	toolPrompt, _ := promptRepo.GetActive(ctx, p.BusinessID, "chatbot_tool_instructions")

	var prefix strings.Builder
	if sysPrompt != nil {
		prefix.WriteString(strings.TrimSpace(sysPrompt.Content))
	}
	if faqPrompt != nil {
		prefix.WriteString("\n\n")
		prefix.WriteString(strings.TrimSpace(faqPrompt.Content))
	}
	if toolPrompt != nil {
		prefix.WriteString("\n\n")
		prefix.WriteString(strings.TrimSpace(toolPrompt.Content))
	}

	messages := []openai.ChatMessage{
		{Role: "system", Content: prefix.String()},
		{Role: "system", Content: buildCalendarBlock(time.Now(), 28, cfg.BusinessHours.Timezone)},
	}
	for _, m := range history {
		if m.Role == "user" || m.Role == "assistant" {
			messages = append(messages, openai.ChatMessage{Role: m.Role, Content: m.Content})
		}
	}

	// --- 4. Load credentials & build clients ---
	oaKeyRaw, err := sdk.GetCredential(deps.DB, p.BusinessID, "openai", deps.EncKey)
	if err != nil {
		return fmt.Errorf("load openai cred: %w", err)
	}
	oaClient := openai.NewClient(strings.TrimSpace(oaKeyRaw))

	var pcClient pinecone.Client
	if pcRaw, err := sdk.GetCredential(deps.DB, p.BusinessID, "pinecone", deps.EncKey); err == nil {
		var pcCreds struct {
			APIKey string `json:"api_key"`
			Host   string `json:"host"`
		}
		if json.Unmarshal([]byte(pcRaw), &pcCreds) == nil {
			pcClient = pinecone.NewClient(pinecone.Credentials{
				APIKey: pcCreds.APIKey, IndexHost: pcCreds.Host,
			})
		}
	}

	// --- 5. Tool-call loop ---
	maxIter := cfg.Chatbot.MaxToolIterations
	if maxIter <= 0 {
		maxIter = 3
	}
	chatModel := cfg.Chatbot.Model
	if chatModel == "" {
		chatModel = "gpt-4o-mini"
	}
	temp := cfg.Chatbot.Temperature
	if temp == 0 {
		temp = 0.3
	}

	var finalText string
	for iter := 0; iter < maxIter; iter++ {
		req := openai.ChatRequest{
			Model:       chatModel,
			Messages:    messages,
			Tools:       chatbotTools(),
			ToolChoice:  "auto",
			Temperature: temp,
		}
		resp, err := oaClient.ChatCompletion(ctx, req)
		if err != nil {
			return fmt.Errorf("openai chat completion: %w", err)
		}
		if len(resp.Choices) == 0 {
			return fmt.Errorf("openai returned no choices")
		}
		choice := resp.Choices[0].Message

		// If the model called tools, execute them and loop.
		if len(choice.ToolCalls) > 0 {
			// Append the assistant turn with its tool calls.
			messages = append(messages, choice)
			for _, tc := range choice.ToolCalls {
				result := executeChatbotTool(ctx, tc, lead, pcClient, oaClient, cfg)
				messages = append(messages, openai.ChatMessage{
					Role:       "tool",
					ToolCallID: tc.ID,
					Content:    result,
				})
				// The save_leads_data tool may have mutated the lead —
				// reload it for the next iteration so the model sees the
				// latest state.
				if tc.Function.Name == "save_leads_data" {
					if fresh, ferr := leadRepo.GetByID(ctx, lead.ID); ferr == nil {
						lead = fresh
					}
				}
			}
			continue
		}

		// No tool calls → this is the user-facing reply.
		finalText = strings.TrimSpace(choice.Content)
		break
	}

	if finalText == "" {
		// Model produced no text even after tool calls (e.g. hit max iterations).
		// Send a graceful fallback so the lead isn't left hanging.
		finalText = "Mohon maaf kak, mohon ditunggu sebentar ya."
		deps.Log.Warnf("chatbot turn: empty final text for lead %s, sent fallback", lead.ID)
	}

	// --- 6. Send the reply via Gupshup ---
	if err := sendGupshupReply(ctx, p.BusinessID, lead.Phone, finalText); err != nil {
		deps.Log.Errorf("chatbot turn: gupshup send failed: %v", err)
		// Don't fail the task — the reply is still stored below, operator
		// can retry manually. Failing would duplicate the LLM round.
	}

	// --- 7. Persist assistant message ---
	_ = msgRepo.Insert(ctx, &model.LeadMessage{
		BusinessID: p.BusinessID,
		LeadID:     lead.ID,
		Direction:  "outbound",
		Role:       "assistant",
		Content:    finalText,
		CreatedAt:  time.Now(),
	})

	// --- 8. Update chatbot_states (increments chat_total, resets remarks) ---
	sessionKey := ""
	if state != nil {
		sessionKey = state.SessionKey
	}
	newState, err := stateRepo.RecordTurn(ctx, lead.ID, p.BusinessID, sessionKey, time.Now())
	if err != nil {
		deps.Log.Errorf("chatbot turn: record turn: %v", err)
	}

	// --- 9. Enqueue intent classifier (always) + spam classifier (if threshold crossed) ---
	if deps.Asynq != nil {
		if task, err := newIntentClassifyTask(p.BusinessID, lead.ID); err == nil {
			_, _ = deps.Asynq.EnqueueContext(ctx, task)
		}
		spamThreshold := cfg.SpamClassifyThreshold
		if spamThreshold <= 0 {
			spamThreshold = 5
		}
		if newState != nil && newState.ChatTotal >= spamThreshold {
			if task, err := newSpamClassifyTask(p.BusinessID, lead.ID); err == nil {
				_, _ = deps.Asynq.EnqueueContext(ctx, task)
			}
		}
	}

	return nil
}

// sendGupshupReply loads the per-project Gupshup credential and dispatches
// a WhatsApp text message synchronously.
func sendGupshupReply(ctx context.Context, businessID uuid.UUID, phone, message string) error {
	credRaw, err := sdk.GetCredential(deps.DB, businessID, "gupshup", deps.EncKey)
	if err != nil {
		return fmt.Errorf("load gupshup cred: %w", err)
	}
	var creds gupshup.Credentials
	if err := json.Unmarshal([]byte(credRaw), &creds); err != nil {
		return fmt.Errorf("parse gupshup cred: %w", err)
	}
	client := gupshup.NewClient(creds)
	return client.SendText(ctx, phone, message)
}

// chatbotTools returns the OpenAI tool declarations exposed to the model.
func chatbotTools() []openai.Tool {
	return []openai.Tool{
		{
			Type: "function",
			Function: openai.ToolDeclaration{
				Name:        "property_knowledge",
				Description: "Cari informasi tentang properti di knowledge base (spesifikasi, harga, fasilitas, lokasi, cicilan, akses). WAJIB gunakan sebelum menjawab pertanyaan detail tentang properti yang tidak ada di FAQ. Jangan gunakan untuk FAQ yang sudah dijawab.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"query": map[string]any{
							"type":        "string",
							"description": "Pertanyaan atau topik yang dicari di knowledge base properti.",
						},
					},
					"required": []string{"query"},
				},
			},
		},
		{
			Type: "function",
			Function: openai.ToolDeclaration{
				Name:        "save_leads_data",
				Description: "WAJIB dipanggil setiap user menyebutkan TANGGAL atau JAM visit, ATAU tingkat ketertarikan (Tertarik Site Visit/Warm/Cold). Menyimpan data lead ke database. Tidak boleh menampilkan output tool ke user.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"nama":     map[string]any{"type": "string", "description": "Nama user jika diketahui"},
						"hp":       map[string]any{"type": "string", "description": "Nomor HP user"},
						"email":    map[string]any{"type": "string", "description": "Email user jika disebutkan"},
						"tanggal":  map[string]any{"type": "string", "description": "Tanggal visit (1-31), kosong jika belum ada"},
						"bulan":    map[string]any{"type": "string", "description": "Bulan visit dalam bahasa Indonesia (Januari-Desember)"},
						"tahun":    map[string]any{"type": "string", "description": "Tahun visit (4 digit)"},
						"jam":      map[string]any{"type": "string", "description": "Jam visit format HH:mm (24h)"},
						"summary":  map[string]any{"type": "string", "description": "Ringkasan singkat kata-kata user"},
						"interest": map[string]any{"type": "string", "description": "Tertarik Site Visit (memberikan jadwal) (Hot Leads) | tertarik di informasikan dulu (Warm) | tidak mau atau tidak tertarik (Cold Leads)"},
					},
					"required": []string{"interest", "summary"},
				},
			},
		},
	}
}

// executeChatbotTool runs the handler for a single tool call and returns
// the stringified result that gets appended as a tool message in the next
// OpenAI round.
func executeChatbotTool(
	ctx context.Context,
	tc openai.ToolCall,
	lead *model.Lead,
	pcClient pinecone.Client,
	oaClient openai.Client,
	cfg statemachine.FlowConfig,
) string {
	switch tc.Function.Name {
	case "property_knowledge":
		var args struct {
			Query string `json:"query"`
		}
		_ = json.Unmarshal([]byte(tc.Function.Arguments), &args)
		if pcClient == nil || strings.TrimSpace(args.Query) == "" {
			return "Informasi tidak tersedia di knowledge base."
		}
		embModel := cfg.Chatbot.EmbeddingModel
		if embModel == "" {
			embModel = "text-embedding-3-small"
		}
		vec, err := oaClient.Embed(ctx, embModel, args.Query)
		if err != nil {
			return "Gagal mengambil data dari knowledge base."
		}
		topK := cfg.Chatbot.PineconeTopK
		if topK <= 0 {
			topK = 5
		}
		matches, err := pcClient.Query(ctx, vec, topK)
		if err != nil || len(matches) == 0 {
			return "Tidak ada informasi yang relevan di knowledge base."
		}
		var sb strings.Builder
		for i, m := range matches {
			sb.WriteString(fmt.Sprintf("[%d] ", i+1))
			if txt, ok := m.Metadata["text"].(string); ok {
				sb.WriteString(txt)
			} else if content, ok := m.Metadata["content"].(string); ok {
				sb.WriteString(content)
			}
			sb.WriteString("\n")
		}
		return sb.String()

	case "save_leads_data":
		var args struct {
			Nama     string `json:"nama"`
			HP       string `json:"hp"`
			Email    string `json:"email"`
			Tanggal  string `json:"tanggal"`
			Bulan    string `json:"bulan"`
			Tahun    string `json:"tahun"`
			Jam      string `json:"jam"`
			Summary  string `json:"summary"`
			Interest string `json:"interest"`
		}
		_ = json.Unmarshal([]byte(tc.Function.Arguments), &args)
		if err := applySaveLeadsDataV2(ctx, lead, args); err != nil {
			deps.Log.Errorf("save_leads_data failed for lead %s: %v", lead.ID, err)
			return `{"status":"error"}`
		}
		return `{"status":"ok"}`
	}
	return "Tool tidak dikenali."
}

// applySaveLeadsDataV2 is the correct Anandaya-parity implementation of
// the save_leads_data tool. It:
//   - sets lead.Interest to the tool's interest argument
//   - parses the Indonesian date components into svs_date and sets it
//     ONLY when the interest is "Tertarik Site Visit..." (Hot Leads)
//   - prepends a newest-on-top timestamped note to lead.Summary
//
// Interest2 is NOT written here — that's the intent classifier's job.
func applySaveLeadsDataV2(ctx context.Context, lead *model.Lead, args struct {
	Nama     string `json:"nama"`
	HP       string `json:"hp"`
	Email    string `json:"email"`
	Tanggal  string `json:"tanggal"`
	Bulan    string `json:"bulan"`
	Tahun    string `json:"tahun"`
	Jam      string `json:"jam"`
	Summary  string `json:"summary"`
	Interest string `json:"interest"`
}) error {
	patch := statemachine.Patch{}

	if strings.TrimSpace(args.Interest) != "" {
		v := args.Interest
		patch.Interest = &v
	}

	// Hot Leads → parse svs_date.
	if strings.Contains(strings.ToLower(args.Interest), "tertarik site visit") &&
		args.Tanggal != "" && args.Bulan != "" && args.Jam != "" {
		tahun := args.Tahun
		if tahun == "" {
			tahun = fmt.Sprintf("%d", time.Now().Year())
		}
		if svs, err := statemachine.ParseIndoDate(args.Tanggal, args.Bulan, tahun, args.Jam); err == nil && svs != nil {
			patch.SvsDate = svs
		}
	}

	// Newest-on-top summary append with Jakarta timestamp.
	if strings.TrimSpace(args.Summary) != "" {
		jakarta, err := time.LoadLocation("Asia/Jakarta")
		if err != nil {
			jakarta = time.UTC
		}
		stamp := time.Now().In(jakarta).Format("2006-01-02 15:04:05")
		note := fmt.Sprintf("%s --- %s", stamp, strings.TrimSpace(args.Summary))
		var merged string
		if strings.TrimSpace(lead.Summary) == "" {
			merged = note
		} else {
			merged = note + "\n\n" + lead.Summary
		}
		patch.Summary = &merged
	}

	if patch.IsEmpty() {
		return nil
	}

	leadRepo := repo.NewLeadRepo(deps.DB)
	_, err := leadRepo.Transition(ctx, lead.ID, lead.Version, patch, nil, repo.AuditEntry{
		Actor:     "chatbot.save_leads_data",
		EventType: "save_leads_data_tool",
		Changes: map[string]any{
			"interest": args.Interest,
			"summary":  args.Summary,
			"visit":    fmt.Sprintf("%s %s %s %s", args.Tanggal, args.Bulan, args.Tahun, args.Jam),
		},
	})
	return err
}

// buildCalendarBlock generates an Indonesian 28-day reference calendar
// for injection into the chatbot system prompt.
func buildCalendarBlock(now time.Time, horizonDays int, tz string) string {
	if horizonDays <= 0 {
		horizonDays = 28
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		loc = time.UTC
	}
	now = now.In(loc)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)

	days := []string{"Minggu", "Senin", "Selasa", "Rabu", "Kamis", "Jumat", "Sabtu"}
	months := []string{"", "Januari", "Februari", "Maret", "April", "Mei", "Juni", "Juli", "Agustus", "September", "Oktober", "November", "Desember"}

	var sb strings.Builder
	sb.WriteString("KALENDER REFERENSI:\n")
	for i := 0; i < horizonDays; i++ {
		d := today.AddDate(0, 0, i)
		var label string
		switch {
		case i == 0:
			label = "Hari Ini"
		case i == 1:
			label = "Besok"
		case i < 7:
			label = "Minggu Ini"
		case i < 14:
			label = "Minggu Depan"
		case i < 21:
			label = "2 Minggu Lagi"
		default:
			label = "3 Minggu Lagi"
		}
		sb.WriteString(fmt.Sprintf("- %s: %s, %d %s %d\n",
			label, days[d.Weekday()], d.Day(), months[d.Month()], d.Year()))
	}
	return sb.String()
}
