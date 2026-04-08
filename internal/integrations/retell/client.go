package retell

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Call represents an outbound call
type Call struct {
	ID                  string       `json:"call_id"`
	ToNumber            string       `json:"to_number"`
	FromNumber          string       `json:"from_number"`
	DisconnectionReason string       `json:"disconnection_reason"`
	Analysis            CallAnalysis `json:"call_analysis"`
	Metadata            map[string]string `json:"metadata"`
}

// CallAnalysis holds sentiment and custom extraction logic from Retell
type CallAnalysis struct {
	CallSummary        string                 `json:"call_summary"`
	CustomAnalysisData map[string]interface{} `json:"custom_analysis_data"`
}

// WebhookPayload represents the structure sent by Retell when a call ends
type WebhookPayload struct {
	Event string `json:"event"`
	Call  Call   `json:"call"`
}

// Client represents the Retell AI integration SDK
type Client interface {
	CreateCall(ctx context.Context, agentID, toNumber, fromNumber string, dynamicVars map[string]interface{}, metadata map[string]string) (*Call, error)
	Verify(ctx context.Context) error
}

type retellClient struct {
	apiKey     string
	httpClient *http.Client
}

// NewClient creates a Retell API client authenticated with the given API key.
func NewClient(apiKey string) Client {
	return &retellClient{
		apiKey: apiKey,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *retellClient) CreateCall(ctx context.Context, agentID, toNumber, fromNumber string, dynamicVars map[string]interface{}, metadata map[string]string) (*Call, error) {
	body := map[string]interface{}{
		"from_number":                    fromNumber,
		"to_number":                      toNumber,
		"call_type":                      "phone_call",
		"override_agent_id":              agentID,
		"retell_llm_dynamic_variables":   dynamicVars,
		"metadata":                       metadata,
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.retellai.com/v2/create-phone-call", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("retell API call failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("retell API returned %d: %s", resp.StatusCode, string(respBody))
	}

	var call Call
	if err := json.Unmarshal(respBody, &call); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &call, nil
}

func (c *retellClient) Verify(ctx context.Context) error {
	// Attempt to list agents as a lightweight check
	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.retellai.com/list-agents", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("connection failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("invalid API key")
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("retell API returned status %d", resp.StatusCode)
	}

	return nil
}
