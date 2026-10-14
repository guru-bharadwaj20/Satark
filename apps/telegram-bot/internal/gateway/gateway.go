// Package gateway is the telegram-bot client for gateway-api.
package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"
)

// Reason mirrors gateway-api's Reason.
type Reason struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Weight  int    `json:"weight"`
	Source  string `json:"source"`
}

// UPIIntent mirrors gateway-api's UPIIntent.
type UPIIntent struct {
	PayeeAddress string `json:"payee_address"`
	PayeeName    string `json:"payee_name"`
	Amount       string `json:"amount"`
	Summary      string `json:"summary"`
}

// QRResult mirrors gateway-api's QRResult.
type QRResult struct {
	Kind string     `json:"kind"`
	UPI  *UPIIntent `json:"upi"`
	URL  string     `json:"url"`
}

// ScanResponse mirrors gateway-api's ScanResponse.
type ScanResponse struct {
	ScanID    string     `json:"scan_id"`
	Verdict   string     `json:"verdict"`
	RiskScore int        `json:"risk_score"`
	Reasons   []Reason   `json:"reasons"`
	NextSteps []string   `json:"next_steps"`
	Lang      string     `json:"lang"`
	QR        []QRResult `json:"qr"`
	Partial   bool       `json:"partial"`
}

// ErrUnavailable means gateway-api could not produce a verdict right now.
var ErrUnavailable = errors.New("gateway unavailable")

// ErrBadInput means gateway-api rejected the input (e.g. unreadable image).
var ErrBadInput = errors.New("gateway rejected input")

// Client calls gateway-api.
type Client struct {
	BaseURL string
	Timeout time.Duration
	HTTP    *http.Client
}

// New builds a client.
func New(baseURL string, timeout time.Duration) *Client {
	return &Client{BaseURL: strings.TrimRight(baseURL, "/"), Timeout: timeout, HTTP: &http.Client{}}
}

// ScanText checks a text message.
func (c *Client) ScanText(ctx context.Context, text, lang, senderType string) (ScanResponse, error) {
	body, err := json.Marshal(map[string]string{"text": text, "lang": lang, "sender_type": senderType})
	if err != nil {
		return ScanResponse{}, err
	}
	return c.post(ctx, "/v1/scan/text", "application/json", bytes.NewReader(body))
}

// ScanQR checks a QR image. caption is optional accompanying text.
func (c *Client) ScanQR(ctx context.Context, image []byte, caption, lang string) (ScanResponse, error) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("image", "photo.jpg")
	if err != nil {
		return ScanResponse{}, err
	}
	if _, err := fw.Write(image); err != nil {
		return ScanResponse{}, err
	}
	_ = mw.WriteField("lang", lang)
	if caption != "" {
		_ = mw.WriteField("context", caption)
	}
	if err := mw.Close(); err != nil {
		return ScanResponse{}, err
	}
	return c.post(ctx, "/v1/scan/qr", mw.FormDataContentType(), &buf)
}

func (c *Client) post(ctx context.Context, path, ct string, body io.Reader) (ScanResponse, error) {
	var out ScanResponse
	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+path, body)
	if err != nil {
		return out, err
	}
	req.Header.Set("Content-Type", ct)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return out, fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusOK:
	case resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusRequestEntityTooLarge:
		return out, fmt.Errorf("%w: HTTP %d", ErrBadInput, resp.StatusCode)
	default:
		return out, fmt.Errorf("%w: HTTP %d", ErrUnavailable, resp.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out); err != nil {
		return out, fmt.Errorf("%w: decode: %w", ErrUnavailable, err)
	}
	return out, nil
}
