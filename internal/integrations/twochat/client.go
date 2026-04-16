// Package twochat is a minimal client for 2Chat's WhatsApp API.
//
// The leadflow engine uses 2Chat for internal-team messaging only:
//   - Developer/sales WA group dispatch (hot lead handoff)
//   - Internal alerts (e.g. empty-name notifications)
//   - Valid-number lookups before a handoff
//
// Customer-facing WhatsApp (chatbot replies, bridging messages, final
// messages) flows through Gupshup — see internal/integrations/gupshup.
// The split mirrors the Anandaya n8n topology.
package twochat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/workflow-builder/core/internal/integrations"
)

const (
	defaultBaseURL = "https://api.p.2chat.io/open/whatsapp"
	defaultTimeout = 30 * time.Second
)

// Client is the 2Chat integration interface.
type Client interface {
	// SendText sends a plain WhatsApp text message to either a phone
	// number or a group UUID. 2Chat's send-message endpoint accepts
	// both forms transparently.
	SendText(ctx context.Context, toNumberOrGroup, message string) error

	// CheckNumber returns whether a phone number is registered on
	// WhatsApp. Used as the valid-number gate before handing a lead
	// off to the developer team.
	CheckNumber(ctx context.Context, phone string) (bool, error)

	// Verify does a lightweight credential probe. Used by the admin
	// wizard's "Verify" button.
	Verify(ctx context.Context) error
}

// Credentials holds a per-project 2Chat API key and from-number.
type Credentials struct {
	APIKey     string `json:"api_key"`
	FromNumber string `json:"from_number"` // e.g. "+628123456789"
	BaseURL    string `json:"base_url,omitempty"`
}

type twoChatClient struct {
	creds Credentials
	base  string
	http  *http.Client
}

// NewClient constructs a 2Chat client bound to per-project credentials.
func NewClient(creds Credentials) Client {
	base := creds.BaseURL
	if base == "" {
		base = defaultBaseURL
	}
	return &twoChatClient{
		creds: creds,
		base:  base,
		http:  &http.Client{Timeout: defaultTimeout},
	}
}

// ---- SendText ----

type sendTextRequest struct {
	ToNumber   string `json:"to_number"`
	FromNumber string `json:"from_number"`
	Text       string `json:"text"`
}

func (c *twoChatClient) SendText(ctx context.Context, toNumberOrGroup, message string) error {
	if c.creds.APIKey == "" {
		return fmt.Errorf("%w: missing 2chat api key", integrations.ErrAuth)
	}
	body, err := json.Marshal(sendTextRequest{
		ToNumber:   toNumberOrGroup,
		FromNumber: c.creds.FromNumber,
		Text:       message,
	})
	if err != nil {
		return fmt.Errorf("%w: marshal 2chat send: %v", integrations.ErrPermanent, err)
	}
	req, err := http.NewRequestWithContext(ctx, "POST", c.base+"/send-message", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("%w: build 2chat request: %v", integrations.ErrPermanent, err)
	}
	req.Header.Set("X-User-API-Key", c.creds.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%w: 2chat network: %v", integrations.ErrTransient, err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	return classify(resp.StatusCode, respBody)
}

// ---- CheckNumber ----

type checkNumberResponse struct {
	OnWhatsapp bool `json:"on_whatsapp"`
}

func (c *twoChatClient) CheckNumber(ctx context.Context, phone string) (bool, error) {
	if c.creds.APIKey == "" {
		return false, fmt.Errorf("%w: missing 2chat api key", integrations.ErrAuth)
	}
	u := fmt.Sprintf("%s/check-number/%s", c.base, url.PathEscape(phone))
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return false, fmt.Errorf("%w: %v", integrations.ErrPermanent, err)
	}
	req.Header.Set("X-User-API-Key", c.creds.APIKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return false, fmt.Errorf("%w: %v", integrations.ErrTransient, err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if err := classify(resp.StatusCode, respBody); err != nil {
		return false, err
	}
	var out checkNumberResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		// Some 2Chat endpoints wrap the response — be forgiving: if we
		// can't decode, assume number is valid rather than blocking.
		return true, nil
	}
	return out.OnWhatsapp, nil
}

// ---- Verify ----

func (c *twoChatClient) Verify(ctx context.Context) error {
	if c.creds.APIKey == "" {
		return fmt.Errorf("%w: missing 2chat api key", integrations.ErrAuth)
	}
	// The /numbers endpoint is a lightweight credential probe (lists the
	// account's available WA numbers). A 200 means the key is valid.
	req, err := http.NewRequestWithContext(ctx, "GET", c.base+"/get-numbers", nil)
	if err != nil {
		return fmt.Errorf("%w: %v", integrations.ErrPermanent, err)
	}
	req.Header.Set("X-User-API-Key", c.creds.APIKey)
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
