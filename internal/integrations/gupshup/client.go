// Package gupshup is a minimal outbound client for Gupshup's WhatsApp send API.
//
// Inbound messages from Gupshup flow through n8n's existing webhook forwarder
// into POST /api/webhooks/chat-inbound/:slug (not this package) — only outbound
// sends live here.
package gupshup

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/workflow-builder/core/internal/integrations"
)

const (
	defaultBaseURL = "https://mediaapi.smsgupshup.com/GatewayAPI/rest"
	defaultTimeout = 30 * time.Second
)

// Client is the outbound WA send interface.
type Client interface {
	SendText(ctx context.Context, toPhone, message string) error
	Verify(ctx context.Context) error
}

// Credentials holds the per-project Gupshup HSM/WA API credentials.
type Credentials struct {
	UserID    string
	Password  string
	AppName   string
	SrcNumber string // from-number (per Gupshup account)
}

type gupshupClient struct {
	creds Credentials
	base  string
	http  *http.Client
}

// NewClient constructs a Gupshup client with the given per-project credentials.
func NewClient(creds Credentials) Client {
	return &gupshupClient{
		creds: creds,
		base:  defaultBaseURL,
		http:  &http.Client{Timeout: defaultTimeout},
	}
}

// SendText dispatches a plain WhatsApp text message to `toPhone`.
// The message is assumed to be either a pre-approved HSM template or a
// session message (< 24h since last inbound); policy selection is the
// caller's responsibility.
func (c *gupshupClient) SendText(ctx context.Context, toPhone, message string) error {
	if c.creds.UserID == "" || c.creds.Password == "" {
		return fmt.Errorf("%w: missing gupshup credentials", integrations.ErrAuth)
	}

	form := url.Values{}
	form.Set("method", "SENDMESSAGE")
	form.Set("userid", c.creds.UserID)
	form.Set("password", c.creds.Password)
	form.Set("send_to", toPhone)
	form.Set("msg", message)
	form.Set("msg_type", "TEXT")
	form.Set("auth_scheme", "plain")
	form.Set("v", "1.1")
	form.Set("format", "text")
	if c.creds.AppName != "" {
		form.Set("app_name", c.creds.AppName)
	}
	if c.creds.SrcNumber != "" {
		form.Set("source", c.creds.SrcNumber)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.base, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("%w: build gupshup request: %v", integrations.ErrPermanent, err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%w: gupshup network: %v", integrations.ErrTransient, err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)

	// Gupshup's v1.1 REST endpoint returns 200 even for some errors, with
	// an "error|" prefix in the response body. Classify by body content.
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return fmt.Errorf("%w: gupshup auth rejected: %s", integrations.ErrAuth, bodyStr)
	}
	if resp.StatusCode >= 500 {
		return fmt.Errorf("%w: gupshup %d: %s", integrations.ErrTransient, resp.StatusCode, bodyStr)
	}
	if strings.HasPrefix(strings.TrimSpace(bodyStr), "error|") {
		// Auth errors in-body usually say "authentication failed" or "invalid credentials".
		lower := strings.ToLower(bodyStr)
		if strings.Contains(lower, "auth") || strings.Contains(lower, "credentials") {
			return fmt.Errorf("%w: gupshup: %s", integrations.ErrAuth, bodyStr)
		}
		return fmt.Errorf("%w: gupshup: %s", integrations.ErrPermanent, bodyStr)
	}
	return nil
}

// Verify does a lightweight credential check by sending no-op parameters.
// Gupshup doesn't have a dedicated ping endpoint, so this is a best-effort
// credential probe. The admin wizard uses this for the "Verify" button.
func (c *gupshupClient) Verify(ctx context.Context) error {
	if c.creds.UserID == "" || c.creds.Password == "" {
		return fmt.Errorf("%w: missing gupshup credentials", integrations.ErrAuth)
	}
	// balance endpoint (HSM API) returns account balance; fast & cheap.
	form := url.Values{}
	form.Set("method", "BALANCE_ENQUIRY")
	form.Set("userid", c.creds.UserID)
	form.Set("password", c.creds.Password)
	form.Set("format", "text")
	form.Set("auth_scheme", "plain")

	req, err := http.NewRequestWithContext(ctx, "POST", c.base, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("%w: %v", integrations.ErrPermanent, err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", integrations.ErrTransient, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)

	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return fmt.Errorf("%w: gupshup: %s", integrations.ErrAuth, bodyStr)
	}
	if strings.HasPrefix(strings.TrimSpace(bodyStr), "error|") {
		return fmt.Errorf("%w: gupshup: %s", integrations.ErrAuth, bodyStr)
	}
	return nil
}
