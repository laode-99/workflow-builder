package statemachine

import (
	"encoding/json"
	"fmt"
)

// FlowConfig is the flat, per-project configuration surface for the leadflow
// engine. It is stored as jsonb on businesses.config. Any field left unset in
// the database override JSON falls back to the Defaults() value.
type FlowConfig struct {
	AttemptLimit            int              `json:"attempt_limit"`
	CallRetryGapHours       int              `json:"call_retry_gap_hours"`
	MaxOutGraceHours        int              `json:"max_out_grace_hours"`
	RemarksDelayHours       int              `json:"remarks_delay_hours"`
	SpamClassifyThreshold   int              `json:"spam_classify_threshold"`
	VoicemailShortcutToLast bool             `json:"voicemail_shortcut_to_last"`
	BusinessHours           BusinessHoursCfg `json:"business_hours"`
	ChannelsEnabled         []string         `json:"channels_enabled"`
	IngestionCron           string           `json:"ingestion_cron"`
	LanguageCode            string           `json:"language_code"`
	CRM                     CRMCfg           `json:"crm"`
	Chatbot                 ChatbotCfg       `json:"chatbot"`
}

// BusinessHoursCfg defines the per-project outbound-dispatch window.
type BusinessHoursCfg struct {
	Start    string `json:"start"`    // "HH:MM"
	End      string `json:"end"`      // "HH:MM", exclusive
	Timezone string `json:"timezone"` // IANA name
}

// CRMCfg describes how the project's leads map to its CRM.
type CRMCfg struct {
	Provider      string `json:"provider"`       // "leadsquared" for MVP
	TagFilter     string `json:"tag_filter"`     // mx_Custom_1 value for ingestion filtering
	ActivityEvent int    `json:"activity_event"` // LeadSquared event ID for ingestion
}

// ChatbotCfg contains chatbot agent tuning knobs.
type ChatbotCfg struct {
	Model                 string  `json:"model"`
	Temperature           float64 `json:"temperature"`
	WindowSize            int     `json:"window_size"`
	MaxToolIterations     int     `json:"max_tool_iterations"`
	MaxTurnTokens         int     `json:"max_turn_tokens"`
	PineconeTopK          int     `json:"pinecone_top_k"`
	CalendarHorizonDays   int     `json:"calendar_horizon_days"`
	EmbeddingModel        string  `json:"embedding_model"`
	IntentClassifierModel string  `json:"intent_classifier_model"`
	SpamClassifierModel   string  `json:"spam_classifier_model"`
}

// Defaults returns the baseline FlowConfig matching the Anandaya reference
// implementation. New projects begin with these values and override only
// what differs.
func Defaults() FlowConfig {
	return FlowConfig{
		AttemptLimit:            5,
		CallRetryGapHours:       3,
		MaxOutGraceHours:        24,
		RemarksDelayHours:       5,
		SpamClassifyThreshold:   5,
		VoicemailShortcutToLast: true,
		BusinessHours: BusinessHoursCfg{
			Start:    "07:00",
			End:      "20:00",
			Timezone: "Asia/Jakarta",
		},
		ChannelsEnabled: []string{"call", "whatsapp"},
		IngestionCron:   "*/2 7-19 * * *",
		LanguageCode:    "id-ID",
		CRM: CRMCfg{
			Provider:      "leadsquared",
			ActivityEvent: 12002,
		},
		Chatbot: ChatbotCfg{
			Model:                 "gpt-4o-mini",
			Temperature:           0.3,
			WindowSize:            4,
			MaxToolIterations:     3,
			MaxTurnTokens:         8000,
			PineconeTopK:          5,
			CalendarHorizonDays:   28,
			EmbeddingModel:        "text-embedding-3-small",
			IntentClassifierModel: "gpt-4o-mini",
			SpamClassifierModel:   "gpt-4o-mini",
		},
	}
}

// LoadConfig merges a JSON override (typically from businesses.config)
// into the defaults. Fields absent from the override JSON keep their default
// values. Empty or "{}" input returns the pure defaults.
func LoadConfig(overrideJSON []byte) (FlowConfig, error) {
	cfg := Defaults()
	s := string(overrideJSON)
	if len(overrideJSON) == 0 || s == "{}" || s == "null" {
		return cfg, nil
	}
	if err := json.Unmarshal(overrideJSON, &cfg); err != nil {
		return cfg, fmt.Errorf("parse flow config: %w", err)
	}
	return cfg, nil
}
