// Package analyzer orchestrates the link-intelligence checks that url-intel
// runs against a submitted URL and aggregates their findings into a report.
package analyzer

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
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

// ThreatCheck marks checks that must run even for allowlisted hosts,
// such as threat-feed lookups (official sites can be compromised).
type ThreatCheck interface {
	Check
	ThreatIntel()
}

// Allowlist recognises official domains.
type Allowlist interface {
	Match(host string) (reason string, ok bool)
}

// Preparer turns the raw input into a Target (normalisation, expansion, ...).
// Preparers run in order before any check.
type Preparer interface {
	Prepare(ctx context.Context, t *Target) ([]Signal, error)
}

// Report is the aggregated result returned to callers.
type Report struct {
	Target   Target    `json:"target"`
	Signals  []Signal  `json:"signals"`
	Findings []Finding `json:"findings"`
	Score    int       `json:"score"`
	// Allowlisted explains why the host is considered official, if it is.
	// Only threat-intel checks run for allowlisted hosts.
	Allowlisted string    `json:"allowlisted,omitempty"`
	CheckedAt   time.Time `json:"checked_at"`
}

// ErrInvalidInput is returned when the input cannot be analysed.
var ErrInvalidInput = errors.New("invalid input")

// Analyzer runs preparers then checks.
type Analyzer struct {
	preparers []Preparer
	checks    []Check
	allow     Allowlist
	now       func() time.Time

	sem          chan struct{}
	checkTimeout time.Duration
	observe      func(check string, d time.Duration, outcome string)
}

// Defaults for the worker pool.
const (
	DefaultWorkers      = 64
	DefaultCheckTimeout = 5 * time.Second
)

// ErrCheckTimeout is recorded when a check exceeds its time budget.
var ErrCheckTimeout = errors.New("check timed out")

// TimeoutHinter lets a check ask for its own time budget.
type TimeoutHinter interface {
	Timeout() time.Duration
}

// WithWorkers bounds how many checks run concurrently across all requests.
func WithWorkers(n int) Option {
	return func(a *Analyzer) {
		if n > 0 {
			a.sem = make(chan struct{}, n)
		}
	}
}

// WithCheckTimeout sets the default per-check time budget.
func WithCheckTimeout(d time.Duration) Option {
	return func(a *Analyzer) {
		if d > 0 {
			a.checkTimeout = d
		}
	}
}

// WithObserver receives every check's duration and outcome
// ("ok", "error", "timeout", "rejected") for metrics.
func WithObserver(f func(check string, d time.Duration, outcome string)) Option {
	return func(a *Analyzer) { a.observe = f }
}

// WithAllowlist sets the official-domain allowlist.
func WithAllowlist(l Allowlist) Option {
	return func(a *Analyzer) { a.allow = l }
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
	a := &Analyzer{now: time.Now, sem: make(chan struct{}, DefaultWorkers), checkTimeout: DefaultCheckTimeout}
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
	checks := a.checks
	if a.allow != nil {
		if reason, ok := a.allow.Match(t.Host); ok {
			rep.Allowlisted = reason
			rep.Signals = append(rep.Signals, Signal{ID: "allowlist.official", Severity: SeverityInfo, Weight: -40,
				Detail: "link goes to an " + reason})
			checks = nil
			for _, c := range a.checks {
				if _, ok := c.(ThreatCheck); ok {
					checks = append(checks, c)
				}
			}
		}
	}
	rep.Findings = a.runChecks(ctx, t, checks)
	for _, f := range rep.Findings {
		rep.Signals = append(rep.Signals, f.Signals...)
	}
	rep.Target = *t
	rep.Score = Score(rep.Signals)
	return rep, nil
}

// runChecks runs checks concurrently on the shared worker pool, each under
// its own deadline. Results keep the order of checks. A check that ignores
// its context is abandoned at the deadline and reported as timed out.
func (a *Analyzer) runChecks(ctx context.Context, t *Target, checks []Check) []Finding {
	out := make([]Finding, len(checks))
	var wg sync.WaitGroup
	for i, c := range checks {
		wg.Add(1)
		go func(i int, c Check) {
			defer wg.Done()
			out[i] = a.runOne(ctx, t, c)
		}(i, c)
	}
	wg.Wait()
	return out
}

func (a *Analyzer) runOne(ctx context.Context, t *Target, c Check) Finding {
	start := time.Now()
	finish := func(f Finding, outcome string) Finding {
		f.Check = c.Name()
		f.DurationMS = time.Since(start).Milliseconds()
		if a.observe != nil {
			a.observe(f.Check, time.Since(start), outcome)
		}
		return f
	}

	select {
	case a.sem <- struct{}{}:
	case <-ctx.Done():
		return finish(Finding{Error: "not started: " + ctx.Err().Error()}, "rejected")
	}
	timeout := a.checkTimeout
	if h, ok := c.(TimeoutHinter); ok && h.Timeout() > 0 {
		timeout = h.Timeout()
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	done := make(chan Finding, 1)
	go func() {
		defer func() { <-a.sem }()
		defer func() {
			if r := recover(); r != nil {
				done <- Finding{Error: fmt.Sprintf("check panicked: %v", r)}
			}
		}()
		done <- c.Run(cctx, t)
	}()

	select {
	case f := <-done:
		if f.Error != "" {
			if cctx.Err() == context.DeadlineExceeded {
				return finish(Finding{Error: ErrCheckTimeout.Error()}, "timeout")
			}
			return finish(f, "error")
		}
		return finish(f, "ok")
	case <-cctx.Done():
		return finish(Finding{Error: ErrCheckTimeout.Error()}, "timeout")
	}
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
