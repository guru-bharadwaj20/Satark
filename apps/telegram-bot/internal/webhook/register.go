package webhook

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// DefaultAPIBase is the Telegram Bot API endpoint.
const DefaultAPIBase = "https://api.telegram.org"

// Client calls the Bot API methods that manage the webhook.
type Client struct {
	Token   string
	APIBase string // defaults to DefaultAPIBase
	HTTP    *http.Client
}

// SetWebhookParams mirrors the setWebhook parameters this bot uses.
type SetWebhookParams struct {
	URL                string
	SecretToken        string
	AllowedUpdates     []string
	MaxConnections     int
	DropPendingUpdates bool
}

// WebhookInfo is the subset of getWebhookInfo the bot reports on.
type WebhookInfo struct {
	URL                  string   `json:"url"`
	PendingUpdateCount   int      `json:"pending_update_count"`
	LastErrorDate        int64    `json:"last_error_date"`
	LastErrorMessage     string   `json:"last_error_message"`
	MaxConnections       int      `json:"max_connections"`
	AllowedUpdates       []string `json:"allowed_updates"`
	HasCustomCertificate bool     `json:"has_custom_certificate"`
}

// APIError is a Bot API error response.
type APIError struct {
	Method      string
	Code        int
	Description string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("telegram %s: %d %s", e.Method, e.Code, e.Description)
}

// SetWebhook registers the webhook URL with Telegram.
func (c *Client) SetWebhook(ctx context.Context, p SetWebhookParams) error {
	u, err := url.Parse(p.URL)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return errors.New("webhook: URL must be an absolute https URL")
	}
	if !validSecret(p.SecretToken) {
		return errors.New("webhook: invalid secret token")
	}
	form := url.Values{}
	form.Set("url", p.URL)
	form.Set("secret_token", p.SecretToken)
	if len(p.AllowedUpdates) > 0 {
		b, _ := json.Marshal(p.AllowedUpdates)
		form.Set("allowed_updates", string(b))
	}
	if p.MaxConnections > 0 {
		form.Set("max_connections", strconv.Itoa(p.MaxConnections))
	}
	if p.DropPendingUpdates {
		form.Set("drop_pending_updates", "true")
	}
	return c.call(ctx, "setWebhook", form, nil)
}

// DeleteWebhook removes the webhook, for example to go back to long polling.
func (c *Client) DeleteWebhook(ctx context.Context, dropPending bool) error {
	form := url.Values{}
	if dropPending {
		form.Set("drop_pending_updates", "true")
	}
	return c.call(ctx, "deleteWebhook", form, nil)
}

// GetWebhookInfo returns the current webhook status.
func (c *Client) GetWebhookInfo(ctx context.Context) (WebhookInfo, error) {
	var info WebhookInfo
	err := c.call(ctx, "getWebhookInfo", url.Values{}, &info)
	return info, err
}

func (c *Client) call(ctx context.Context, method string, form url.Values, result any) error {
	if c.Token == "" {
		return errors.New("webhook: bot token is required")
	}
	base := c.APIBase
	if base == "" {
		base = DefaultAPIBase
	}
	hc := c.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 15 * time.Second}
	}
	endpoint := strings.TrimRight(base, "/") + "/bot" + c.Token + "/" + method
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := hc.Do(req)
	if err != nil {
		// Strip the URL from the error: it contains the bot token.
		var uerr *url.Error
		if errors.As(err, &uerr) {
			return fmt.Errorf("telegram %s: %w", method, uerr.Err)
		}
		return fmt.Errorf("telegram %s: request failed", method)
	}
	defer resp.Body.Close()

	var env struct {
		OK          bool            `json:"ok"`
		Result      json.RawMessage `json:"result"`
		ErrorCode   int             `json:"error_code"`
		Description string          `json:"description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return fmt.Errorf("telegram %s: decode response (HTTP %d): %w", method, resp.StatusCode, err)
	}
	if !env.OK {
		code := env.ErrorCode
		if code == 0 {
			code = resp.StatusCode
		}
		return &APIError{Method: method, Code: code, Description: env.Description}
	}
	if result != nil {
		if err := json.Unmarshal(env.Result, result); err != nil {
			return fmt.Errorf("telegram %s: decode result: %w", method, err)
		}
	}
	return nil
}

// Mode selects how the bot receives updates.
type Mode string

const (
	ModePolling Mode = "polling"
	ModeWebhook Mode = "webhook"
)

// EnvConfig is the webhook configuration read from the environment.
type EnvConfig struct {
	Mode       Mode
	PublicURL  string // TELEGRAM_WEBHOOK_URL, e.g. https://bot.satark.example.in/telegram/webhook
	Secret     string // TELEGRAM_WEBHOOK_SECRET
	ListenAddr string // TELEGRAM_WEBHOOK_LISTEN, default :8082
	Path       string // derived from PublicURL
}

// FromEnv reads TELEGRAM_MODE (polling|webhook, default polling),
// TELEGRAM_WEBHOOK_URL, TELEGRAM_WEBHOOK_SECRET and TELEGRAM_WEBHOOK_LISTEN.
func FromEnv() (EnvConfig, error) {
	return parseEnv(os.Getenv)
}

func parseEnv(getenv func(string) string) (EnvConfig, error) {
	cfg := EnvConfig{
		Mode:       Mode(strings.ToLower(strings.TrimSpace(getenv("TELEGRAM_MODE")))),
		PublicURL:  strings.TrimSpace(getenv("TELEGRAM_WEBHOOK_URL")),
		Secret:     getenv("TELEGRAM_WEBHOOK_SECRET"),
		ListenAddr: getenv("TELEGRAM_WEBHOOK_LISTEN"),
	}
	if cfg.Mode == "" {
		cfg.Mode = ModePolling
	}
	if cfg.ListenAddr == "" {
		cfg.ListenAddr = ":8082"
	}
	switch cfg.Mode {
	case ModePolling:
		return cfg, nil
	case ModeWebhook:
	default:
		return cfg, fmt.Errorf("webhook: unknown TELEGRAM_MODE %q", cfg.Mode)
	}
	u, err := url.Parse(cfg.PublicURL)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return cfg, errors.New("webhook: TELEGRAM_WEBHOOK_URL must be an absolute https URL")
	}
	if !validSecret(cfg.Secret) {
		return cfg, errors.New("webhook: TELEGRAM_WEBHOOK_SECRET must be 1-256 characters of A-Z, a-z, 0-9, _ or -")
	}
	cfg.Path = u.Path
	if cfg.Path == "" {
		cfg.Path = "/"
	}
	return cfg, nil
}
