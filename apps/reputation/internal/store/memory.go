package store

import (
	"context"
	"sync"
	"time"
)

// Memory is an in-process Store for tests and local development.
type Memory struct {
	mu         sync.Mutex
	reports    []Report
	delistings []Delisting
	nextID     int64
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

func (m *Memory) DeleteSubject(_ context.Context, kind, subject string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	kept, n := m.reports[:0], 0
	for _, e := range m.reports {
		if e.Kind == kind && e.SubjectHash == subject {
			n++
			continue
		}
		kept = append(kept, e)
	}
	m.reports = kept
	return n, nil
}

func (m *Memory) CreateDelisting(_ context.Context, d Delisting) (Delisting, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nextID++
	d.ID, d.Status = m.nextID, DelistPending
	m.delistings = append(m.delistings, d)
	return d, nil
}

func (m *Memory) ListDelistings(_ context.Context, status string) ([]Delisting, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Delisting
	for _, d := range m.delistings {
		if status == "" || d.Status == status {
			out = append(out, d)
		}
	}
	return out, nil
}

func (m *Memory) DecideDelisting(_ context.Context, id int64, status, note string, at time.Time) (Delisting, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, d := range m.delistings {
		if d.ID == id && d.Status == DelistPending {
			d.Status, d.ReviewNote, d.DecidedAt = status, note, &at
			m.delistings[i] = d
			return d, nil
		}
	}
	return Delisting{}, ErrNotFound
}

func (m *Memory) Ping(context.Context) error { return nil }
func (m *Memory) Close()                     {}
