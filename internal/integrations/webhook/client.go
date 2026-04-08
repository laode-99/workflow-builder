package webhook

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

type Client struct {
	httpClient *http.Client
}

func NewClient() *Client {
	return &Client{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Trigger sends a POST request with an optional JSON body
func (c *Client) Trigger(ctx context.Context, url string, payload []byte) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(payload))
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/json")
	// You can add more security headers here (e.g. signature for n8n)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return string(body), fmt.Errorf("http error: %d", resp.StatusCode)
	}

	return string(body), nil
}
