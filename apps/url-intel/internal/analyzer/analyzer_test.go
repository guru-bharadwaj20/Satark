package analyzer

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fakeCheck struct {
	name   string
	weight int
	threat bool
}

func (f fakeCheck) Name() string { return f.name }
func (f fakeCheck) Run(context.Context, *Target) Finding {
	return Finding{Signals: []Signal{{ID: f.name, Weight: f.weight}}}
}

type threatCheck struct{ fakeCheck }

func (threatCheck) ThreatIntel() {}

type allow struct{}

func (allow) Match(h string) (string, bool) {
	return "official .gov.in domain", strings.HasSuffix(h, ".gov.in")
}

func TestAnalyzeScoresAndAllowlist(t *testing.T) {
	a := New(WithAllowlist(allow{}), WithChecks(
		fakeCheck{name: "a", weight: 30},
		fakeCheck{name: "b", weight: 90},
		threatCheck{fakeCheck{name: "feed", weight: 10}},
	))
	rep, err := a.Analyze(context.Background(), "https://x.example/")
	if err != nil {
		t.Fatal(err)
	}
	if rep.Score != 100 || len(rep.Findings) != 3 || rep.Allowlisted != "" {
		t.Fatalf("%+v", rep)
	}

	rep, err = a.Analyze(context.Background(), "https://uidai.gov.in/")
	if err != nil {
		t.Fatal(err)
	}
	if rep.Allowlisted == "" || len(rep.Findings) != 1 || rep.Findings[0].Check != "feed" {
		t.Fatalf("allowlisted: %+v", rep)
	}
	if rep.Score != 0 {
		t.Fatalf("score %d", rep.Score)
	}
}

func TestAnalyzeInvalid(t *testing.T) {
	if _, err := New().Analyze(context.Background(), "  "); err != ErrInvalidInput {
		t.Fatal(err)
	}
}

type sleepy struct {
	d          time.Duration
	ignoresCtx bool
}

func (sleepy) Name() string { return "sleepy" }
func (s sleepy) Run(ctx context.Context, _ *Target) Finding {
	if s.ignoresCtx {
		time.Sleep(s.d)
		return Finding{}
	}
	select {
	case <-time.After(s.d):
		return Finding{}
	case <-ctx.Done():
		return Finding{Error: ctx.Err().Error()}
	}
}

type panicky struct{}

func (panicky) Name() string                         { return "panicky" }
func (panicky) Run(context.Context, *Target) Finding { panic("boom") }

func TestPerCheckTimeoutAndOrder(t *testing.T) {
	var outcomes sync.Map
	a := New(WithCheckTimeout(50*time.Millisecond), WithWorkers(4),
		WithObserver(func(c string, _ time.Duration, o string) { outcomes.Store(c, o) }),
		WithChecks(fakeCheck{name: "fast", weight: 1}, sleepy{d: time.Second}, sleepy{d: time.Second, ignoresCtx: true}, panicky{}))
	start := time.Now()
	rep, err := a.Analyze(context.Background(), "https://x.example/")
	if err != nil {
		t.Fatal(err)
	}
	if el := time.Since(start); el > 500*time.Millisecond {
		t.Fatalf("checks not bounded: %v", el)
	}
	if rep.Findings[0].Check != "fast" || rep.Findings[0].Error != "" {
		t.Fatalf("%+v", rep.Findings[0])
	}
	for _, i := range []int{1, 2} {
		if rep.Findings[i].Error != ErrCheckTimeout.Error() {
			t.Fatalf("finding %d: %+v", i, rep.Findings[i])
		}
	}
	if !strings.Contains(rep.Findings[3].Error, "panicked") {
		t.Fatalf("%+v", rep.Findings[3])
	}
	if o, _ := outcomes.Load("fast"); o != "ok" {
		t.Fatal(o)
	}
}

func TestWorkerPoolBoundsConcurrency(t *testing.T) {
	var cur, peak atomic.Int32
	c := funcCheck(func() {
		n := cur.Add(1)
		for {
			p := peak.Load()
			if n <= p || peak.CompareAndSwap(p, n) {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
		cur.Add(-1)
	})
	checks := make([]Check, 10)
	for i := range checks {
		checks[i] = c
	}
	a := New(WithWorkers(3), WithChecks(checks...))
	if _, err := a.Analyze(context.Background(), "https://x.example/"); err != nil {
		t.Fatal(err)
	}
	if p := peak.Load(); p > 3 {
		t.Fatalf("peak concurrency %d > 3", p)
	}
}

type funcCheck func()

func (funcCheck) Name() string { return "func" }
func (f funcCheck) Run(context.Context, *Target) Finding {
	f()
	return Finding{}
}
