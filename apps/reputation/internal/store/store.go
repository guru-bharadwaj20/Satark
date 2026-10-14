// Package store persists community reports. Only HMAC hashes of subjects
// and reporters are stored, never raw phone numbers, VPAs or user IDs.
package store

import (
	"context"
	"time"
)

// Report is a single community report about a subject.
type Report struct {
	Kind         string
	SubjectHash  string
	ReporterHash string
	Category     string
	CreatedAt    time.Time
}

// Summary aggregates reports for one subject.
type Summary struct {
	DistinctReporters int
	Categories        map[string]int
	FirstSeen         time.Time
	LastSeen          time.Time
}

// Store is the persistence interface used by the API.
type Store interface {
	// AddReport records r. It returns false when the same reporter has
	// already reported the same subject (the report is then a no-op).
	AddReport(ctx context.Context, r Report) (bool, error)
	// Summary returns the aggregate for a subject; zero value if unknown.
	Summary(ctx context.Context, kind, subjectHash string) (Summary, error)
	Ping(ctx context.Context) error
	Close()
}
