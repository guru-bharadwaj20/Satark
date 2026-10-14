package ratelimit

import (
	"testing"
	"time"
)

func TestAllow(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	l := New(func() time.Time { return now }, Window{2, time.Hour}, Window{3, 24 * time.Hour})
	for i := 0; i < 2; i++ {
		if ok, _ := l.Allow("a"); !ok {
			t.Fatalf("call %d denied", i)
		}
	}
	ok, wait := l.Allow("a")
	if ok || wait != time.Hour {
		t.Fatalf("want deny with 1h wait, got %v %v", ok, wait)
	}
	if ok, _ := l.Allow("b"); !ok {
		t.Fatal("keys must be independent")
	}
	now = now.Add(time.Hour)
	if ok, _ := l.Allow("a"); !ok {
		t.Fatal("hourly window should have reset")
	}
	if ok, _ := l.Allow("a"); ok {
		t.Fatal("daily limit of 3 should apply")
	}
	now = now.Add(24 * time.Hour)
	if ok, _ := l.Allow("a"); !ok {
		t.Fatal("daily window should have reset")
	}
}
