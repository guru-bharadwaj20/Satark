// Package urlscan implements the gateway's POST /v1/scan/url endpoint. It
// forwards the link to url-intel and turns the signal report into a
// user-facing verdict. The package only depends on the standard library so
// it can be mounted on any router:
//
//	mux.Handle("POST /v1/scan/url", urlscan.NewHandler(urlscan.NewClient(baseURL, 15*time.Second)))
package urlscan

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

// DefaultURLIntelBase is the in-cluster url-intel address.
const DefaultURLIntelBase = "http://url-intel.satark.svc.cluster.local:8081"

const (
	maxRequestBytes  = 8 << 10
	maxResponseBytes = 2 << 20
	maxURLLength     = 2048
	maxReasons       = 5
)

// Verdict buckets. There is deliberately no "safe" verdict: the best
// outcome Satark reports is that no red flags were found.
const (
	VerdictNoRedFlags = "no_red_flags"
	VerdictSuspicious = "suspicious"
	VerdictDangerous  = "dangerous"
)

// Signal mirrors url-intel's signal.
type Signal struct {
	ID       string `json:"id"`
	Severity string `json:"severity"`
	Weight   int    `json:"weight"`
	Detail   string `json:"detail,omitempty"`
}

// Report is the subset of url-intel's report used by the gateway.
type Report struct {
	Target struct {
		Raw           string   `json:"raw"`
		Normalized    string   `json:"normalized"`
		FinalURL      string   `json:"final_url"`
		RedirectChain []string `json:"redirect_chain,omitempty"`
	} `json:"target"`
	Signals     []Signal  `json:"signals"`
	Score       int       `json:"score"`
	Allowlisted string    `json:"allowlisted,omitempty"`
	CheckedAt   time.Time `json:"checked_at"`
}

// ErrInvalidURL is returned when url-intel rejects the input.
var ErrInvalidURL = errors.New("invalid url")

// Analyzer is what the handler needs from url-intel.
type Analyzer interface {
	Analyze(ctx context.Context, url string) (*Report, error)
}

// Client calls url-intel over HTTP.
type Client struct {
	Base string
	HTTP *http.Client
}

// NewClient returns a url-intel client.
func NewClient(base string, timeout time.Duration) *Client {
	if base == "" {
		base = DefaultURLIntelBase
	}
	return &Client{Base: strings.TrimRight(base, "/"), HTTP: &http.Client{Timeout: timeout}}
}

// Analyze implements Analyzer.
func (c *Client) Analyze(ctx context.Context, u string) (*Report, error) {
	body, _ := json.Marshal(map[string]string{"url": u})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Base+"/v1/analyze", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusUnprocessableEntity || resp.StatusCode == http.StatusBadRequest:
		var e struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(io.LimitReader(resp.Body, 4<<10)).Decode(&e)
		return nil, fmt.Errorf("%w: %s", ErrInvalidURL, strings.TrimPrefix(e.Error, "invalid input: "))
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("url-intel returned status %d", resp.StatusCode)
	}
	var rep Report
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(&rep); err != nil {
		return nil, fmt.Errorf("decode url-intel response: %w", err)
	}
	return &rep, nil
}

// Response is returned to clients of the gateway.
type Response struct {
	Verdict       string    `json:"verdict"`
	Headline      string    `json:"headline"`
	Score         int       `json:"score"`
	URL           string    `json:"url"`
	FinalURL      string    `json:"final_url"`
	RedirectChain []string  `json:"redirect_chain,omitempty"`
	Reasons       []string  `json:"reasons"`
	Advice        string    `json:"advice"`
	Official      bool      `json:"official"`
	CheckedAt     time.Time `json:"checked_at"`
}

// Classify maps a url-intel report to a verdict. Any single high-severity
// signal with a large weight is enough for "dangerous", regardless of the
// summed score, so a strong allowlist discount cannot hide a feed hit.
func Classify(rep *Report) string {
	for _, s := range rep.Signals {
		if s.Severity == "high" && s.Weight >= 60 {
			return VerdictDangerous
		}
	}
	switch {
	case rep.Score >= 60:
		return VerdictDangerous
	case rep.Score >= 25:
		return VerdictSuspicious
	default:
		return VerdictNoRedFlags
	}
}

// BuildResponse turns a report into the public response.
func BuildResponse(rep *Report) Response {
	v := Classify(rep)
	r := Response{
		Verdict: v, Score: rep.Score, URL: rep.Target.Normalized, FinalURL: rep.Target.FinalURL,
		RedirectChain: rep.Target.RedirectChain, Official: rep.Allowlisted != "", CheckedAt: rep.CheckedAt,
		Reasons: reasons(rep.Signals),
	}
	switch v {
	case VerdictDangerous:
		r.Headline = "Dangerous link"
		r.Advice = "Do not open this link, enter OTPs or passwords, or install anything from it. Report fraud at cybercrime.gov.in or call 1930."
	case VerdictSuspicious:
		r.Headline = "Suspicious link"
		r.Advice = "Be careful. Verify with the organisation through its official app or website before sharing any details."
	default:
		r.Headline = "No red flags found"
		r.Advice = "We found no warning signs, but no check is perfect. Never share OTPs, PINs or passwords over a link."
	}
	return r
}

func reasons(sig []Signal) []string {
	s := make([]Signal, 0, len(sig))
	for _, x := range sig {
		if x.Weight > 0 && x.Detail != "" {
			s = append(s, x)
		}
	}
	sort.SliceStable(s, func(i, j int) bool { return s[i].Weight > s[j].Weight })
	out := []string{}
	for i := 0; i < len(s) && len(out) < maxReasons; i++ {
		out = append(out, s[i].Detail)
	}
	return out
}

// Handler serves POST /v1/scan/url.
type Handler struct {
	A       Analyzer
	Timeout time.Duration
}

// NewHandler returns a Handler with a 15s upstream budget.
func NewHandler(a Analyzer) *Handler { return &Handler{A: a, Timeout: 15 * time.Second} }

type request struct {
	URL string `json:"url"`
}

// ServeHTTP implements http.Handler.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req request
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequestBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, `body must be JSON like {"url": "https://..."}`)
		return
	}
	req.URL = strings.TrimSpace(req.URL)
	if req.URL == "" || len(req.URL) > maxURLLength {
		writeError(w, http.StatusBadRequest, "url is required and must be at most 2048 characters")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.Timeout)
	defer cancel()
	rep, err := h.A.Analyze(ctx, req.URL)
	switch {
	case errors.Is(err, ErrInvalidURL):
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	case errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded):
		writeError(w, http.StatusGatewayTimeout, "link analysis timed out, please try again")
		return
	case err != nil:
		writeError(w, http.StatusBadGateway, "link analysis is temporarily unavailable")
		return
	}
	writeJSON(w, http.StatusOK, BuildResponse(rep))
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
