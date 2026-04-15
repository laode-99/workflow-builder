// Package openai is a minimal client for OpenAI's chat completions,
// function calling, and embeddings endpoints. It wraps every error into
// one of integrations.ErrTransient / ErrPermanent / ErrAuth so callers
// can make retry decisions uniformly.
//
// The leadflow engine uses a single global OpenAI credential (not per-project)
// because the customer has consolidated billing. Per-project tuning happens
// through FlowConfig.chatbot (model, temperature, window size, etc.).
package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/workflow-builder/core/internal/integrations"
)

const (
	baseURL          = "https://api.openai.com/v1"
	defaultTimeout   = 60 * time.Second
	chatEndpoint     = "/chat/completions"
	embedEndpoint    = "/embeddings"
)

// Client is the exported interface implemented by openaiClient. Callers
// should depend on this interface, not the concrete type, so tests can
// inject fakes.
type Client interface {
	ChatCompletion(ctx context.Context, req ChatRequest) (*ChatResponse, error)
	Embed(ctx context.Context, model, input string) ([]float32, error)
	Verify(ctx context.Context) error
}

type openaiClient struct {
	apiKey string
	http   *http.Client
}

// NewClient constructs an OpenAI client bound to a single API key.
func NewClient(apiKey string) Client {
	return &openaiClient{
		apiKey: apiKey,
		http:   &http.Client{Timeout: defaultTimeout},
	}
}

// ---- Chat completions ----

// ChatMessage is a single turn in the conversation.
type ChatMessage struct {
	Role       string      `json:"role"` // system | user | assistant | tool
	Content    string      `json:"content,omitempty"`
	Name       string      `json:"name,omitempty"`
	ToolCallID string      `json:"tool_call_id,omitempty"`
	ToolCalls  []ToolCall  `json:"tool_calls,omitempty"`
}

// ToolCall is the model asking to invoke one of the declared tools.
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"` // always "function"
	Function ToolFunction `json:"function"`
}

// ToolFunction is the function call embedded inside a ToolCall.
type ToolFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // JSON-encoded object
}

// Tool declares a callable function available to the model.
type Tool struct {
	Type     string           `json:"type"` // always "function"
	Function ToolDeclaration  `json:"function"`
}

// ToolDeclaration is the schema for a single exposed function.
type ToolDeclaration struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

// ChatRequest is the subset of the chat.completions request that the
// leadflow engine needs.
type ChatRequest struct {
	Model       string        `json:"model"`
	Messages    []ChatMessage `json:"messages"`
	Tools       []Tool        `json:"tools,omitempty"`
	ToolChoice  string        `json:"tool_choice,omitempty"` // "auto" | "none"
	Temperature float64       `json:"temperature"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
}

// ChatResponse is the subset of the chat.completions response used by
// the chatbot agent loop.
type ChatResponse struct {
	ID      string   `json:"id"`
	Model   string   `json:"model"`
	Choices []Choice `json:"choices"`
	Usage   Usage    `json:"usage"`
}

// Choice is one completion candidate.
type Choice struct {
	Index        int         `json:"index"`
	Message      ChatMessage `json:"message"`
	FinishReason string      `json:"finish_reason"` // stop | tool_calls | length | ...
}

// Usage is the token-usage accounting for a single call.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
	// Prompt caching fields from the OpenAI response (when present).
	PromptTokensDetails struct {
		CachedTokens int `json:"cached_tokens"`
	} `json:"prompt_tokens_details"`
}

// ChatCompletion calls the chat.completions endpoint.
func (c *openaiClient) ChatCompletion(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("%w: marshal chat request: %v", integrations.ErrPermanent, err)
	}
	var resp ChatResponse
	if err := c.do(ctx, "POST", chatEndpoint, body, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ---- Embeddings ----

type embedRequest struct {
	Model string `json:"model"`
	Input string `json:"input"`
}

type embedResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
}

// Embed returns a single embedding vector for the given input text.
// Used by the Pinecone RAG tool at chatbot turn time.
func (c *openaiClient) Embed(ctx context.Context, model, input string) ([]float32, error) {
	body, err := json.Marshal(embedRequest{Model: model, Input: input})
	if err != nil {
		return nil, fmt.Errorf("%w: marshal embed request: %v", integrations.ErrPermanent, err)
	}
	var resp embedResponse
	if err := c.do(ctx, "POST", embedEndpoint, body, &resp); err != nil {
		return nil, err
	}
	if len(resp.Data) == 0 {
		return nil, fmt.Errorf("%w: empty embedding response", integrations.ErrPermanent)
	}
	return resp.Data[0].Embedding, nil
}

// Verify checks whether the API key works with a lightweight call.
// Used by the admin wizard's credential-verify step.
func (c *openaiClient) Verify(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "GET", baseURL+"/models", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", integrations.ErrTransient, err)
	}
	defer resp.Body.Close()
	return classifyStatus(resp.StatusCode, resp.Body)
}

// do is the shared HTTP plumbing: marshal + auth + call + classify + unmarshal.
func (c *openaiClient) do(ctx context.Context, method, endpoint string, body []byte, out any) error {
	req, err := http.NewRequestWithContext(ctx, method, baseURL+endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("%w: build request: %v", integrations.ErrPermanent, err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%w: openai call: %v", integrations.ErrTransient, err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if err := classifyStatus(resp.StatusCode, bytes.NewReader(respBody)); err != nil {
		return err
	}

	if out != nil {
		if err := json.Unmarshal(respBody, out); err != nil {
			return fmt.Errorf("%w: unmarshal response: %v", integrations.ErrPermanent, err)
		}
	}
	return nil
}

// classifyStatus maps an HTTP status code to one of the three integration
// error categories. 2xx returns nil.
func classifyStatus(code int, body io.Reader) error {
	if code >= 200 && code < 300 {
		return nil
	}
	msg := ""
	if body != nil {
		b, _ := io.ReadAll(body)
		msg = string(b)
	}
	switch {
	case code == 401 || code == 403:
		return fmt.Errorf("%w: HTTP %d: %s", integrations.ErrAuth, code, msg)
	case code == 429 || code >= 500:
		return fmt.Errorf("%w: HTTP %d: %s", integrations.ErrTransient, code, msg)
	default:
		return fmt.Errorf("%w: HTTP %d: %s", integrations.ErrPermanent, code, msg)
	}
}

// ExtractToolCalls returns the tool calls from the first choice's message,
// or nil if the model produced a text answer instead. Convenience used by
// the chatbot agent loop.
func ExtractToolCalls(resp *ChatResponse) ([]ToolCall, error) {
	if resp == nil || len(resp.Choices) == 0 {
		return nil, errors.New("empty chat response")
	}
	return resp.Choices[0].Message.ToolCalls, nil
}

// ExtractText returns the text content of the first choice's message.
func ExtractText(resp *ChatResponse) (string, error) {
	if resp == nil || len(resp.Choices) == 0 {
		return "", errors.New("empty chat response")
	}
	return resp.Choices[0].Message.Content, nil
}
