// Package leadsquared is a minimal client for LeadSquared's Opportunity
// Management and Lead Management APIs, covering only the operations the
// leadflow engine needs: fetching new opportunities for ingestion and
// updating the fields that drive CRM sync.
//
// Field mapping (mx_Custom_*) is defined in crm_mapping.go and matches the
// Anandaya reference implementation verbatim. Per-project overrides can be
// added later via FlowConfig.crm.field_mapping.
package leadsquared

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
	defaultBaseURL = "https://api-in21.leadsquared.com/v2"
	defaultTimeout = 30 * time.Second
)

// Client is the interface used by ingestion and CRM sync workers.
type Client interface {
	FetchOpportunitiesByTag(ctx context.Context, tagFilter string, activityEvent int, since time.Time) ([]Opportunity, error)
	UpdateOpportunity(ctx context.Context, opportunityID string, fields []FieldUpdate) error
	GetLeadDetails(ctx context.Context, leadID string) (*LeadDetail, error)
	Verify(ctx context.Context) error
}

// Credentials holds per-project LeadSquared access keys.
type Credentials struct {
	AccessKey string
	SecretKey string
	BaseURL   string // optional override; defaults to defaultBaseURL
}

// Opportunity is the minimal subset of a LeadSquared OpportunityManagement
// record used during ingestion.
type Opportunity struct {
	OpportunityID        string
	RelatedProspectID    string
	CreatedOn            time.Time
	Fields               map[string]string // arbitrary mx_Custom_* fields
}

// LeadDetail is the minimal subset of LeadManagement.svc/Leads.GetById used
// to resolve phone + name for a fetched opportunity.
type LeadDetail struct {
	ID        string
	Phone     string
	FirstName string
	LastName  string
	Email     string
}

// FieldUpdate is one field modification to apply to an opportunity.
type FieldUpdate struct {
	SchemaName string // e.g. "mx_Custom_25"
	Value      string
}

type leadsquaredClient struct {
	creds Credentials
	base  string
	http  *http.Client
}

// NewClient constructs a LeadSquared client.
func NewClient(creds Credentials) Client {
	base := creds.BaseURL
	if base == "" {
		base = defaultBaseURL
	}
	return &leadsquaredClient{
		creds: creds,
		base:  base,
		http:  &http.Client{Timeout: defaultTimeout},
	}
}

// ---- FetchOpportunitiesByTag ----

// FetchOpportunitiesByTag pulls opportunities matching a tag filter
// (mx_Custom_1 value) and activity event ID created since the given time.
func (c *leadsquaredClient) FetchOpportunitiesByTag(ctx context.Context, tagFilter string, activityEvent int, since time.Time) ([]Opportunity, error) {
	// Build filter payload matching the Anandaya n8n reference.
	filter := map[string]any{
		"Parameter": map[string]any{
			"ActivityEvent": activityEvent,
			"FromDate":      since.Format("2006-01-02 15:04:05"),
			"ToDate":         time.Now().Format("2006-01-02 15:04:05"),
		},
		"Columns": map[string]any{
			"Include_CSV": "ProspectOpportunityId,RelatedProspectId,CreatedOn,mx_Custom_1",
		},
		"RemoveNullValues": "false",
	}
	if tagFilter != "" {
		filter["SearchParameter"] = map[string]any{"mx_Custom_1": tagFilter}
	}
	body, err := json.Marshal(filter)
	if err != nil {
		return nil, fmt.Errorf("%w: marshal fetch filter: %v", integrations.ErrPermanent, err)
	}
	endpoint := fmt.Sprintf("%s/OpportunityManagement.svc/Retrieve/BySearchParameter?%s",
		c.base, c.authQuery())

	respBody, err := c.postJSON(ctx, endpoint, body)
	if err != nil {
		return nil, err
	}

	// The response shape is loose; decode defensively.
	var rawResp struct {
		RecordCount int                     `json:"RecordCount"`
		List        []map[string]any        `json:"List"`
	}
	if err := json.Unmarshal(respBody, &rawResp); err != nil {
		return nil, fmt.Errorf("%w: decode opportunities: %v", integrations.ErrPermanent, err)
	}

	var out []Opportunity
	for _, row := range rawResp.List {
		opp := Opportunity{Fields: make(map[string]string)}
		if v, ok := row["ProspectOpportunityId"].(string); ok {
			opp.OpportunityID = v
		}
		if v, ok := row["RelatedProspectId"].(string); ok {
			opp.RelatedProspectID = v
		}
		if v, ok := row["CreatedOn"].(string); ok {
			if t, err := time.Parse("2006-01-02 15:04:05", v); err == nil {
				opp.CreatedOn = t
			}
		}
		for k, v := range row {
			if s, ok := v.(string); ok {
				opp.Fields[k] = s
			}
		}
		out = append(out, opp)
	}
	return out, nil
}

// ---- UpdateOpportunity ----

// UpdateOpportunity applies a set of field updates to an existing opportunity.
func (c *leadsquaredClient) UpdateOpportunity(ctx context.Context, opportunityID string, fields []FieldUpdate) error {
	payload := make([]map[string]string, 0, len(fields))
	for _, f := range fields {
		payload = append(payload, map[string]string{
			"Attribute": f.SchemaName,
			"Value":     f.Value,
		})
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("%w: marshal update: %v", integrations.ErrPermanent, err)
	}
	endpoint := fmt.Sprintf("%s/OpportunityManagement.svc/Update?%s&opportunityId=%s",
		c.base, c.authQuery(), url.QueryEscape(opportunityID))

	_, err = c.postJSON(ctx, endpoint, body)
	return err
}

// ---- GetLeadDetails ----

func (c *leadsquaredClient) GetLeadDetails(ctx context.Context, leadID string) (*LeadDetail, error) {
	endpoint := fmt.Sprintf("%s/LeadManagement.svc/Leads.GetById?%s&id=%s",
		c.base, c.authQuery(), url.QueryEscape(leadID))

	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", integrations.ErrPermanent, err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", integrations.ErrTransient, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if err := classify(resp.StatusCode, body); err != nil {
		return nil, err
	}

	var rawList []map[string]any
	if err := json.Unmarshal(body, &rawList); err != nil {
		// Sometimes returns an object; fall back to that.
		var rawObj map[string]any
		if err2 := json.Unmarshal(body, &rawObj); err2 != nil {
			return nil, fmt.Errorf("%w: decode lead details: %v", integrations.ErrPermanent, err)
		}
		rawList = []map[string]any{rawObj}
	}
	if len(rawList) == 0 {
		return nil, fmt.Errorf("%w: lead %s not found", integrations.ErrPermanent, leadID)
	}
	row := rawList[0]
	out := &LeadDetail{ID: leadID}
	if v, ok := row["Phone"].(string); ok {
		out.Phone = v
	}
	if v, ok := row["FirstName"].(string); ok {
		out.FirstName = v
	}
	if v, ok := row["LastName"].(string); ok {
		out.LastName = v
	}
	if v, ok := row["EmailAddress"].(string); ok {
		out.Email = v
	}
	return out, nil
}

// ---- Verify ----

func (c *leadsquaredClient) Verify(ctx context.Context) error {
	if c.creds.AccessKey == "" || c.creds.SecretKey == "" {
		return fmt.Errorf("%w: missing leadsquared credentials", integrations.ErrAuth)
	}
	// Attempt a minimal lead-management call as a credential probe.
	endpoint := fmt.Sprintf("%s/LeadManagement.svc/LeadsMetaData.Get?%s", c.base, c.authQuery())
	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return fmt.Errorf("%w: %v", integrations.ErrPermanent, err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", integrations.ErrTransient, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return classify(resp.StatusCode, body)
}

// ---- HTTP helpers ----

func (c *leadsquaredClient) authQuery() string {
	v := url.Values{}
	v.Set("accessKey", c.creds.AccessKey)
	v.Set("secretKey", c.creds.SecretKey)
	return v.Encode()
}

func (c *leadsquaredClient) postJSON(ctx context.Context, endpoint string, body []byte) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", integrations.ErrPermanent, err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: leadsquared network: %v", integrations.ErrTransient, err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if err := classify(resp.StatusCode, respBody); err != nil {
		return nil, err
	}
	return respBody, nil
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
