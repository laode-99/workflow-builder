// Package pinecone is a minimal client for Pinecone's v1 query API,
// used by the chatbot agent's property_knowledge RAG tool.
package pinecone

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/workflow-builder/core/internal/integrations"
)

const defaultTimeout = 30 * time.Second

// Client is the RAG query interface.
type Client interface {
	Query(ctx context.Context, vector []float32, topK int) ([]Match, error)
	Verify(ctx context.Context) error
}

// Credentials holds per-project Pinecone access details.
type Credentials struct {
	APIKey    string
	IndexHost string // e.g. "https://anandaya-update-2-abc123.svc.us-east-1-aws.pinecone.io"
}

// Match is one result returned by a Pinecone query.
type Match struct {
	ID       string         `json:"id"`
	Score    float32        `json:"score"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

type pineconeClient struct {
	creds Credentials
	http  *http.Client
}

// NewClient constructs a Pinecone client bound to a specific index host.
func NewClient(creds Credentials) Client {
	return &pineconeClient{
		creds: creds,
		http:  &http.Client{Timeout: defaultTimeout},
	}
}

type queryRequest struct {
	Vector          []float32 `json:"vector"`
	TopK            int       `json:"topK"`
	IncludeMetadata bool      `json:"includeMetadata"`
	IncludeValues   bool      `json:"includeValues"`
}

type queryResponse struct {
	Matches []Match `json:"matches"`
}

// Query runs a vector similarity search and returns the top-K matches.
func (c *pineconeClient) Query(ctx context.Context, vector []float32, topK int) ([]Match, error) {
	if c.creds.APIKey == "" || c.creds.IndexHost == "" {
		return nil, fmt.Errorf("%w: missing pinecone credentials", integrations.ErrAuth)
	}
	body, err := json.Marshal(queryRequest{
		Vector:          vector,
		TopK:            topK,
		IncludeMetadata: true,
		IncludeValues:   false,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: marshal query: %v", integrations.ErrPermanent, err)
	}
	req, err := http.NewRequestWithContext(ctx, "POST", c.creds.IndexHost+"/query", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", integrations.ErrPermanent, err)
	}
	req.Header.Set("Api-Key", c.creds.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: pinecone network: %v", integrations.ErrTransient, err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if err := classify(resp.StatusCode, respBody); err != nil {
		return nil, err
	}

	var out queryResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		return nil, fmt.Errorf("%w: decode query response: %v", integrations.ErrPermanent, err)
	}
	return out.Matches, nil
}

// Verify makes a minimal describe-index-stats call to confirm the API key
// and index host are valid.
func (c *pineconeClient) Verify(ctx context.Context) error {
	if c.creds.APIKey == "" || c.creds.IndexHost == "" {
		return fmt.Errorf("%w: missing pinecone credentials", integrations.ErrAuth)
	}
	req, err := http.NewRequestWithContext(ctx, "POST", c.creds.IndexHost+"/describe_index_stats", nil)
	if err != nil {
		return fmt.Errorf("%w: %v", integrations.ErrPermanent, err)
	}
	req.Header.Set("Api-Key", c.creds.APIKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", integrations.ErrTransient, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return classify(resp.StatusCode, body)
}

func classify(code int, body []byte) error {
	if code >= 200 && code < 300 {
		return nil
	}
	msg := string(body)
	switch {
	case code == 401 || code == 403:
		return fmt.Errorf("%w: HTTP %d: %s", integrations.ErrAuth, code, msg)
	case code == 429 || code >= 500:
		return fmt.Errorf("%w: HTTP %d: %s", integrations.ErrTransient, code, msg)
	default:
		return fmt.Errorf("%w: HTTP %d: %s", integrations.ErrPermanent, code, msg)
	}
}
