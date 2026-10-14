// Package analyzers contains HTTP clients for the internal analyzer
// services (text-analyzer, qr-decoder). Every call is bounded by a
// context deadline.
package analyzers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"time"
)

// RuleMatch is one text-analyzer rule hit.
type RuleMatch struct {
	ID       string `json:"id"`
	Category string `json:"category"`
	Weight   int    `json:"weight"`
	Reason   string `json:"reason"`
}

// TextResult is the text-analyzer /v1/analyze response.
type TextResult struct {
	RedactedText string         `json:"redacted_text"`
	Redactions   map[string]int `json:"redactions"`
	Matches      []RuleMatch    `json:"matches"`
	Lang         string         `json:"lang"`
}

// QRFlag is a risk flag raised by qr-decoder.
type QRFlag struct {
	Code    string `json:"code"`
	Weight  int    `json:"weight"`
	Message string `json:"message"`
}

// QRUPI is the parsed UPI intent returned by qr-decoder.
type QRUPI struct {
	PayeeAddress string   `json:"payee_address"`
	PayeeName    string   `json:"payee_name"`
	Amount       string   `json:"amount"`
	Currency     string   `json:"currency"`
	Note         string   `json:"note"`
	Direction    string   `json:"direction"`
	Issues       []string `json:"issues"`
	Summary      string   `json:"summary"`
}

// QRCode is one decoded code.
type QRCode struct {
	Format string   `json:"format"`
	Kind   string   `json:"kind"`
	Text   string   `json:"text"`
	UPI    *QRUPI   `json:"upi"`
	Flags  []QRFlag `json:"flags"`
}

// QRResult is the qr-decoder /v1/decode response.
type QRResult struct {
	Codes []QRCode `json:"codes"`
}

// StatusError is returned when an analyzer answers with a non-2xx status.
type StatusError struct {
	Service string
	Status  int
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("%s returned HTTP %d", e.Service, e.Status)
}

// ErrBadInput marks analyzer 4xx responses caused by the user's input.
var ErrBadInput = errors.New("analyzer rejected input")

// Client talks to one analyzer service.
type Client struct {
	name    string
	baseURL string
	timeout time.Duration
	http    *http.Client
}

// NewClient builds a client; timeout bounds each call on top of the caller's context.
func NewClient(name, baseURL string, timeout time.Duration, hc *http.Client) *Client {
	if hc == nil {
		hc = &http.Client{Transport: &http.Transport{
			MaxIdleConnsPerHost:   32,
			IdleConnTimeout:       90 * time.Second,
			ResponseHeaderTimeout: timeout,
		}}
	}
	return &Client{name: name, baseURL: baseURL, timeout: timeout, http: hc}
}

// Name returns the analyzer's service name.
func (c *Client) Name() string { return c.name }

func (c *Client) do(ctx context.Context, path, contentType string, body io.Reader, out any) error {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Accept", "application/json")
	if id, ok := ctx.Value(requestIDKey{}).(string); ok {
		req.Header.Set("X-Request-Id", id)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%s: %w", c.name, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 && resp.StatusCode < 500 && resp.StatusCode != http.StatusTooManyRequests {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
		return fmt.Errorf("%w: %w", ErrBadInput, &StatusError{c.name, resp.StatusCode})
	}
	if resp.StatusCode/100 != 2 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
		return &StatusError{c.name, resp.StatusCode}
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(out); err != nil {
		return fmt.Errorf("%s: decode response: %w", c.name, err)
	}
	return nil
}

type requestIDKey struct{}

// WithRequestID propagates a correlation ID to analyzer calls.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, id)
}

// TextAnalyzer calls text-analyzer.
type TextAnalyzer struct{ *Client }

// Analyze redacts and rule-checks text.
func (t TextAnalyzer) Analyze(ctx context.Context, text, lang string) (TextResult, error) {
	var out TextResult
	body, err := json.Marshal(map[string]string{"text": text, "lang": lang})
	if err != nil {
		return out, err
	}
	err = t.do(ctx, "/v1/analyze", "application/json", bytes.NewReader(body), &out)
	return out, err
}

// QRDecoder calls qr-decoder.
type QRDecoder struct{ *Client }

// Decode decodes QR codes in an image. caption is optional accompanying text.
func (q QRDecoder) Decode(ctx context.Context, image []byte, caption string) (QRResult, error) {
	var out QRResult
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("image", "upload")
	if err != nil {
		return out, err
	}
	if _, err := fw.Write(image); err != nil {
		return out, err
	}
	if caption != "" {
		if err := mw.WriteField("context", caption); err != nil {
			return out, err
		}
	}
	if err := mw.Close(); err != nil {
		return out, err
	}
	err = q.do(ctx, "/v1/decode", mw.FormDataContentType(), &buf, &out)
	return out, err
}
