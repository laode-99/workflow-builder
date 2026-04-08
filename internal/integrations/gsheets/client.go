package gsheets

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/oauth2/google"
	"golang.org/x/oauth2"
)

// Row represents a single record in a spreadsheet
type Row struct {
	data    map[string]string
	headers []string
	index   int
}

func (r *Row) Get(column string) string { return r.data[column] }
func (r *Row) Index() int               { return r.index }
func (r *Row) ToMap() map[string]string { return r.data }

// Client represents the Google Sheets integration SDK
type Client interface {
	ReadRows(ctx context.Context, sheetID string, rangeStr string) ([]*Row, error)
	UpdateRow(ctx context.Context, sheetID string, rowIndex int, updates map[string]interface{}) error
	UpdateRowByMatch(ctx context.Context, sheetID string, rangeStr string, matchColumn string, matchValue string, updates map[string]interface{}) error
	Verify(ctx context.Context) error
}

type gsheetsClient struct {
	httpClient *http.Client
}

// NewClient creates a Google Sheets client authenticated with a Service Account JSON credential.
func NewClient(ctx context.Context, serviceAccountJSON string) (Client, error) {
	// Auto-fix: Sometimes users paste JSON with escaped newlines as literal \n
	// We unmarshal, fix the key, and remarshal to ensure it's valid for Google's SDK.
	var config map[string]interface{}
	if err := json.Unmarshal([]byte(serviceAccountJSON), &config); err != nil {
		return nil, fmt.Errorf("invalid json: %w", err)
	}

	if pk, ok := config["private_key"].(string); ok {
		pk = strings.TrimSpace(pk)
		// Fix literal \n escaping (from env vars or double-encoded JSON)
		pk = strings.ReplaceAll(pk, "\\n", "\n")
		pk = strings.ReplaceAll(pk, "\\r", "")

		// Robust PEM reconstruction: parse line-by-line and rebuild cleanly
		if strings.Contains(pk, "BEGIN PRIVATE KEY") {
			lines := strings.Split(pk, "\n")
			var b64 strings.Builder
			inBody := false
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if strings.Contains(line, "BEGIN PRIVATE KEY") {
					inBody = true
					continue
				}
				if strings.Contains(line, "END PRIVATE KEY") {
					break
				}
				if inBody && line != "" {
					b64.WriteString(line)
				}
			}
			if b64.Len() > 0 {
				raw := b64.String()
				var wrapped strings.Builder
				wrapped.WriteString("-----BEGIN PRIVATE KEY-----\n")
				for i := 0; i < len(raw); i += 64 {
					end := i + 64
					if end > len(raw) {
						end = len(raw)
					}
					wrapped.WriteString(raw[i:end])
					wrapped.WriteByte('\n')
				}
				wrapped.WriteString("-----END PRIVATE KEY-----")
				config["private_key"] = wrapped.String()
			}
		}
	}

	fixedJSON, _ := json.Marshal(config)

	creds, err := google.CredentialsFromJSON(ctx, fixedJSON,
		"https://www.googleapis.com/auth/spreadsheets",
	)
	if err != nil {
		return nil, fmt.Errorf("invalid service account: %w", err)
	}

	return &gsheetsClient{
		httpClient: oauth2.NewClient(ctx, creds.TokenSource),
	}, nil
}

const sheetsAPI = "https://sheets.googleapis.com/v4/spreadsheets"

// ReadRows fetches all rows from a sheet range. First row is treated as headers.
func (c *gsheetsClient) ReadRows(ctx context.Context, sheetID string, rangeStr string) ([]*Row, error) {
	u := fmt.Sprintf("%s/%s/values/%s", sheetsAPI, sheetID, url.PathEscape(rangeStr))

	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sheets API: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("sheets API %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Values [][]string `json:"values"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	if len(result.Values) < 2 {
		return nil, nil // no data rows
	}

	headers := result.Values[0]
	var rows []*Row
	for i, vals := range result.Values[1:] {
		data := make(map[string]string)
		for j, h := range headers {
			if j < len(vals) {
				data[h] = vals[j]
			}
		}
		rows = append(rows, &Row{data: data, headers: headers, index: i + 2}) // sheets are 1-indexed, +1 for header
	}
	return rows, nil
}

// UpdateRow updates a specific row by index.
func (c *gsheetsClient) UpdateRow(ctx context.Context, sheetID string, rowIndex int, updates map[string]interface{}) error {
	// Not used directly, UpdateRowByMatch handles the full flow
	return nil
}

// UpdateRowByMatch finds a row matching column=value and updates it.
func (c *gsheetsClient) UpdateRowByMatch(ctx context.Context, sheetID string, rangeStr string, matchColumn string, matchValue string, updates map[string]interface{}) error {
	// 1. Read all rows to find the matching one
	rows, err := c.ReadRows(ctx, sheetID, rangeStr)
	if err != nil {
		return err
	}

	var targetRow *Row
	for _, r := range rows {
		if r.Get(matchColumn) == matchValue {
			targetRow = r
			break
		}
	}
	if targetRow == nil {
		return fmt.Errorf("no row found where %s = %s", matchColumn, matchValue)
	}

	// 2. Build the update values in column order
	vals := make([]string, len(targetRow.headers))
	for i, h := range targetRow.headers {
		if v, ok := updates[h]; ok {
			vals[i] = fmt.Sprint(v)
		} else {
			vals[i] = targetRow.data[h] // keep existing
		}
	}

	// 3. Construct the range for this specific row
	tabName := strings.Split(rangeStr, "!")[0]
	rowRange := fmt.Sprintf("%s!A%d", tabName, targetRow.index)

	// 4. PUT to Sheets API
	payload := map[string]interface{}{
		"values": [][]string{vals},
	}
	jsonBody, _ := json.Marshal(payload)

	u := fmt.Sprintf("%s/%s/values/%s?valueInputOption=USER_ENTERED", sheetsAPI, sheetID, url.PathEscape(rowRange))
	req, err := http.NewRequestWithContext(ctx, "PUT", u, strings.NewReader(string(jsonBody)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("sheets update: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		return fmt.Errorf("sheets update %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

func (c *gsheetsClient) Verify(ctx context.Context) error {
	// Attempt to call a generic API (e.g. discovery) or just check initialization success.
	// Since NewClient only succeeds if the credentials JSON is valid, we can do a 
	// small request to verify the token works.
	req, err := http.NewRequestWithContext(ctx, "GET", "https://sheets.googleapis.com/$discovery/rest?version=v4", nil)
	if err != nil {
		return err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("connection failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("google API error %d", resp.StatusCode)
	}
	return nil
}

// ParseTime helper for formatting dates
func init() {
	// Set default timezone
	time.Local = time.FixedZone("WIB", 7*3600)
}
