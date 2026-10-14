package store

import (
	"context"
	"sync"
)

// Memory is an in-process Store for tests and local development.
type Memory struct {
	mu      sync.Mutex
	reports []Report
}

// NewMemory returns an empty in-memory store.
func NewMemory() *Memory { return &Memory{} }

func (m *Memory) AddReport(_ context.Context, r Report) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, e := range m.reports {
		if e.Kind == r.Kind && e.SubjectHash == r.SubjectHash && e.ReporterHash == r.ReporterHash {
			return false, nil
		}
	}
	m.reports = append(m.reports, r)
	return true, nil
}

func (m *Memory) Summary(_ context.Context, kind, subject string) (Summary, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := Summary{Categories: map[string]int{}}
	for _, e := range m.reports {
		if e.Kind != kind || e.SubjectHash != subject {
			continue
		}
		s.DistinctReporters++
		s.Categories[e.Category]++
		if s.FirstSeen.IsZero() || e.CreatedAt.Before(s.FirstSeen) {
			s.FirstSeen = e.CreatedAt
		}
		if e.CreatedAt.After(s.LastSeen) {
			s.LastSeen = e.CreatedAt
		}
	}
	return s, nil
}

func (m *Memory) Ping(context.Context) error { return nil }
func (m *Memory) Close()                     {}
