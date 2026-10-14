package report

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Client submits reports to the reputation service
// (default http://reputation.satark.svc:8083).
type Client struct {
	BaseURL string
	HTTP    *http.Client
}

// NewClient returns a Client with a 5s timeout.
func NewClient(baseURL string) *Client {
	return &Client{BaseURL: baseURL, HTTP: &http.Client{Timeout: 5 * time.Second}}
}

// Submit implements Submitter.
func (c *Client) Submit(ctx context.Context, kind, value, category, reporterID string) (string, error) {
	body, _ := json.Marshal(map[string]string{
		"kind": kind, "value": value, "category": category, "reporter_id": reporterID,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/v1/reports", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var out struct {
		Status string `json:"status"`
		Error  string `json:"error"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if resp.StatusCode != http.StatusAccepted {
		return "", fmt.Errorf("reputation: %d %s", resp.StatusCode, out.Error)
	}
	return out.Status, nil
}
