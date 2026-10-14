package analyzer

import (
	"context"
	"strings"
	"testing"
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
