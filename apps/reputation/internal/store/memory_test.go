package store

import (
	"context"
	"testing"
	"time"
)

func TestPurge(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	m := NewMemory()
	m.AddReport(ctx, Report{Kind: "phone", SubjectHash: "a", ReporterHash: "r1", CreatedAt: now.AddDate(-2, 0, 0)})
	m.AddReport(ctx, Report{Kind: "phone", SubjectHash: "a", ReporterHash: "r2", CreatedAt: now})
	for _, age := range []int{-400, -60, -1} {
		d, _ := m.CreateDelisting(ctx, Delisting{Kind: "phone", Contact: "x@example.com"})
		m.DecideDelisting(ctx, d.ID, DelistRejected, "", now.AddDate(0, 0, age))
	}
	m.CreateDelisting(ctx, Delisting{Kind: "phone", Contact: "pending@example.com"})

	res, err := m.Purge(ctx, RetentionPolicy{
		ReportsBefore:        now.AddDate(-1, 0, 0),
		ContactsBefore:       now.AddDate(0, 0, -30),
		DecidedDelistsBefore: now.AddDate(-1, 0, 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	want := PurgeResult{Reports: 1, ContactsCleared: 1, Delistings: 1}
	if res != want {
		t.Fatalf("got %+v want %+v", res, want)
	}
	if s, _ := m.Summary(ctx, "phone", "a"); s.DistinctReporters != 1 {
		t.Fatalf("recent report removed: %+v", s)
	}
	pending, _ := m.ListDelistings(ctx, DelistPending)
	if len(pending) != 1 || pending[0].Contact == "" {
		t.Fatal("pending request contact must be kept")
	}
}
