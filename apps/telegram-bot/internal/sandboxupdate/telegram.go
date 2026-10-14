package sandboxupdate

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Editor edits a message the bot already sent.
type Editor interface {
	EditMessageText(ctx context.Context, chatID, messageID int64, text string) error
}

// RetryAfterError tells the caller to retry after Telegram's flood-control delay.
type RetryAfterError struct{ After time.Duration }

func (e *RetryAfterError) Error() string {
	return fmt.Sprintf("telegram rate limit, retry after %s", e.After)
}

// ErrPermanent marks edits that can never succeed (message deleted, too old, ...).
var ErrPermanent = errors.New("telegram edit failed permanently")

// BotAPI is a minimal Telegram Bot API client for editMessageText.
type BotAPI struct {
	Token   string
	BaseURL string // default https://api.telegram.org
	HTTP    *http.Client
}

type apiResponse struct {
	OK          bool   `json:"ok"`
	ErrorCode   int    `json:"error_code"`
	Description string `json:"description"`
	Parameters  struct {
		RetryAfter int `json:"retry_after"`
	} `json:"parameters"`
}

// EditMessageText implements Editor.
func (b *BotAPI) EditMessageText(ctx context.Context, chatID, messageID int64, text string) error {
	base := b.BaseURL
	if base == "" {
		base = "https://api.telegram.org"
	}
	hc := b.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 10 * time.Second}
	}
	body, _ := json.Marshal(map[string]any{
		"chat_id":                  chatID,
		"message_id":               messageID,
		"text":                     text,
		"parse_mode":               "HTML",
		"disable_web_page_preview": true,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/bot"+b.Token+"/editMessageText", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := hc.Do(req)
	if err != nil {
		// Never include the URL (it contains the token) in the error.
		return fmt.Errorf("telegram request failed: %w", errors.Unwrap(err))
	}
	defer resp.Body.Close()
	var ar apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&ar); err != nil {
		return fmt.Errorf("telegram: decode response (HTTP %d): %w", resp.StatusCode, err)
	}
	if ar.OK {
		return nil
	}
	switch {
	case ar.ErrorCode == http.StatusTooManyRequests:
		return &RetryAfterError{After: time.Duration(max(ar.Parameters.RetryAfter, 1)) * time.Second}
	case strings.Contains(ar.Description, "message is not modified"):
		return nil // already up to date (redelivery)
	case ar.ErrorCode == http.StatusBadRequest || ar.ErrorCode == http.StatusForbidden:
		return fmt.Errorf("%w: %s", ErrPermanent, ar.Description)
	default:
		return fmt.Errorf("telegram error %d: %s", ar.ErrorCode, ar.Description)
	}
}
