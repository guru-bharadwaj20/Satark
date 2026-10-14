// Package analyzer orchestrates the link-intelligence checks that url-intel
// runs against a submitted URL and aggregates their findings into a report.
package analyzer

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"time"
)

// Severity levels used by signals.
const (
	SeverityInfo   = "info"
	SeverityLow    = "low"
	SeverityMedium = "medium"
	SeverityHigh   = "high"
)

// Signal is a single piece of evidence produced by a check.
type Signal struct {
	ID       string `json:"id"`
	Severity string `json:"severity"`
	Weight   int    `json:"weight"`
	Detail   string `json:"detail,omitempty"`
}

// Target is the URL under analysis, as prepared before checks run.
type Target struct {
	Raw           string   `json:"raw"`
	Normalized    string   `json:"normalized"`
	FinalURL      string   `json:"final_url"`
	RedirectChain []string `json:"redirect_chain,omitempty"`

	// URL is the parsed final URL that checks should inspect.
	URL *url.URL `json:"-"`
	// Host is the lower-cased hostname (no port) of URL.
	Host string `json:"-"`
}

// Finding is the outcome of one check.
type Finding struct {
	Check      string   `json:"check"`
	Signals    []Signal `json:"signals,omitempty"`
	Data       any      `json:"data,omitempty"`
	Error      string   `json:"error,omitempty"`
	DurationMS int64    `json:"duration_ms"`
}

// Check is one independent analysis step.
type Check interface {
	Name() string
	Run(ctx context.Context, t *Target) Finding
}

// Preparer turns the raw input into a Target (normalisation, expansion, ...).
// Preparers run in order before any check.
type Preparer interface {
	Prepare(ctx context.Context, t *Target) ([]Signal, error)
}

// Report is the aggregated result returned to callers.
type Report struct {
	Target    Target    `json:"target"`
	Signals   []Signal  `json:"signals"`
	Findings  []Finding `json:"findings"`
	Score     int       `json:"score"`
	CheckedAt time.Time `json:"checked_at"`
}

// ErrInvalidInput is returned when the input cannot be analysed.
var ErrInvalidInput = errors.New("invalid input")

// Analyzer runs preparers then checks.
type Analyzer struct {
	preparers []Preparer
	checks    []Check
	now       func() time.Time
}

// Option configures an Analyzer.
type Option func(*Analyzer)

// WithPreparers appends preparers.
func WithPreparers(p ...Preparer) Option {
	return func(a *Analyzer) { a.preparers = append(a.preparers, p...) }
}

// WithChecks appends checks.
func WithChecks(c ...Check) Option {
	return func(a *Analyzer) { a.checks = append(a.checks, c...) }
}

// New creates an Analyzer.
func New(opts ...Option) *Analyzer {
	a := &Analyzer{now: time.Now}
	for _, o := range opts {
		o(a)
	}
	return a
}

// Analyze runs the full pipeline for raw.
func (a *Analyzer) Analyze(ctx context.Context, raw string) (*Report, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, ErrInvalidInput
	}
	t := &Target{Raw: raw, Normalized: raw, FinalURL: raw}
	rep := &Report{CheckedAt: a.now().UTC()}
	for _, p := range a.preparers {
		sig, err := p.Prepare(ctx, t)
		rep.Signals = append(rep.Signals, sig...)
		if err != nil {
			return nil, err
		}
	}
	if t.URL == nil {
		u, err := url.Parse(t.FinalURL)
		if err != nil || u.Hostname() == "" {
			return nil, ErrInvalidInput
		}
		t.URL = u
	}
	t.Host = strings.ToLower(t.URL.Hostname())
	rep.Findings = a.runChecks(ctx, t)
	for _, f := range rep.Findings {
		rep.Signals = append(rep.Signals, f.Signals...)
	}
	rep.Target = *t
	rep.Score = Score(rep.Signals)
	return rep, nil
}

func (a *Analyzer) runChecks(ctx context.Context, t *Target) []Finding {
	out := make([]Finding, 0, len(a.checks))
	for _, c := range a.checks {
		start := time.Now()
		f := c.Run(ctx, t)
		f.Check = c.Name()
		f.DurationMS = time.Since(start).Milliseconds()
		out = append(out, f)
	}
	return out
}

// Score sums signal weights, clamped to 0..100.
func Score(signals []Signal) int {
	s := 0
	for _, sig := range signals {
		s += sig.Weight
	}
	if s < 0 {
		return 0
	}
	if s > 100 {
		return 100
	}
	return s
}
