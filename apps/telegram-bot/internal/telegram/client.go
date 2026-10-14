// Package telegram is a small, dependency-free client for the subset of
// the Telegram Bot API that Satark uses.
package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DefaultAPIURL is the public Bot API endpoint.
const DefaultAPIURL = "https://api.telegram.org"

// Client calls the Bot API for one bot token.
type Client struct {
	base string // e.g. https://api.telegram.org/bot<token>
	file string // e.g. https://api.telegram.org/file/bot<token>
	http *http.Client
}

// New creates a client. apiURL is normally DefaultAPIURL.
func New(apiURL, token string, hc *http.Client) *Client {
	apiURL = strings.TrimRight(apiURL, "/")
	if hc == nil {
		hc = &http.Client{Timeout: 0} // per-call deadlines come from contexts
	}
	return &Client{
		base: apiURL + "/bot" + token,
		file: apiURL + "/file/bot" + token,
		http: hc,
	}
}

// APIError is an error reported by the Bot API.
type APIError struct {
	Method      string
	Code        int
	Description string
	RetryAfter  int
}

func (e *APIError) Error() string {
	return fmt.Sprintf("telegram %s: %d %s", e.Method, e.Code, e.Description)
}

type envelope struct {
	OK          bool            `json:"ok"`
	Result      json.RawMessage `json:"result"`
	ErrorCode   int             `json:"error_code"`
	Description string          `json:"description"`
	Parameters  *struct {
		RetryAfter int `json:"retry_after"`
	} `json:"parameters"`
}

// call POSTs params as JSON to method and decodes the result into out.
// The token is part of the URL, so URL errors are scrubbed before return.
func (c *Client) call(ctx context.Context, method string, params, out any) error {
	body, err := json.Marshal(params)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/"+method, bytes.NewReader(body))
	if err != nil {
		return scrub(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return scrub(err)
	}
	defer resp.Body.Close()
	var env envelope
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&env); err != nil {
		return fmt.Errorf("telegram %s: decode: %w", method, err)
	}
	if !env.OK {
		e := &APIError{Method: method, Code: env.ErrorCode, Description: env.Description}
		if env.Parameters != nil {
			e.RetryAfter = env.Parameters.RetryAfter
		}
		return e
	}
	if out != nil {
		return json.Unmarshal(env.Result, out)
	}
	return nil
}

func scrub(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		return fmt.Errorf("telegram request failed: %w", ue.Err)
	}
	return err
}

// GetMe returns the bot's own user.
func (c *Client) GetMe(ctx context.Context) (User, error) {
	var u User
	err := c.call(ctx, "getMe", struct{}{}, &u)
	return u, err
}

// GetUpdates long-polls for updates. timeout is the server-side poll time.
func (c *Client) GetUpdates(ctx context.Context, offset int64, timeout time.Duration, allowed []string) ([]Update, error) {
	params := map[string]any{
		"offset":          offset,
		"timeout":         int(timeout.Seconds()),
		"allowed_updates": allowed,
	}
	ctx, cancel := context.WithTimeout(ctx, timeout+10*time.Second)
	defer cancel()
	var ups []Update
	err := c.call(ctx, "getUpdates", params, &ups)
	return ups, err
}

// DeleteWebhook makes sure long polling is allowed.
func (c *Client) DeleteWebhook(ctx context.Context) error {
	return c.call(ctx, "deleteWebhook", map[string]any{"drop_pending_updates": false}, nil)
}

// SendMessage sends a text message.
func (c *Client) SendMessage(ctx context.Context, m SendMessage) (Message, error) {
	var out Message
	err := c.call(ctx, "sendMessage", m, &out)
	return out, err
}

// File is the result of getFile.
type File struct {
	FileID   string `json:"file_id"`
	FileSize int64  `json:"file_size"`
	FilePath string `json:"file_path"`
}

// ErrFileTooLarge is returned when a download exceeds the size limit.
var ErrFileTooLarge = errors.New("telegram file too large")

// DownloadFile resolves fileID with getFile and downloads it, up to limit bytes.
func (c *Client) DownloadFile(ctx context.Context, fileID string, limit int64) ([]byte, error) {
	var f File
	if err := c.call(ctx, "getFile", map[string]string{"file_id": fileID}, &f); err != nil {
		return nil, err
	}
	if f.FileSize > limit {
		return nil, ErrFileTooLarge
	}
	if f.FilePath == "" || strings.Contains(f.FilePath, "..") {
		return nil, errors.New("telegram getFile: missing file path")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.file+"/"+f.FilePath, nil)
	if err != nil {
		return nil, scrub(err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, scrub(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("telegram file download: HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, ErrFileTooLarge
	}
	return data, nil
}
