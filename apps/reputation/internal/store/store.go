// Package store persists community reports. Only HMAC hashes of subjects
// and reporters are stored, never raw phone numbers, VPAs or user IDs.
package store

import (
	"context"
	"errors"
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

// ErrNotFound is returned when a record does not exist.
var ErrNotFound = errors.New("not found")

// Delisting statuses.
const (
	DelistPending  = "pending"
	DelistApproved = "approved"
	DelistRejected = "rejected"
)

// Delisting is a request from the owner of a number, VPA or domain to
// have community reports about it reviewed and removed.
type Delisting struct {
	ID            int64      `json:"id"`
	Kind          string     `json:"kind"`
	SubjectHash   string     `json:"-"`
	SubjectMasked string     `json:"subject"`
	Reason        string     `json:"reason"`
	Contact       string     `json:"contact,omitempty"`
	Status        string     `json:"status"`
	ReviewNote    string     `json:"review_note,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	DecidedAt     *time.Time `json:"decided_at,omitempty"`
}

// Store is the persistence interface used by the API.
type Store interface {
	// AddReport records r. It returns false when the same reporter has
	// already reported the same subject (the report is then a no-op).
	AddReport(ctx context.Context, r Report) (bool, error)
	// Summary returns the aggregate for a subject; zero value if unknown.
	Summary(ctx context.Context, kind, subjectHash string) (Summary, error)
	// DeleteSubject removes every report about a subject (used when a
	// delisting request is approved). It returns the number removed.
	DeleteSubject(ctx context.Context, kind, subjectHash string) (int, error)

	// DeleteReporter removes every report filed by a reporter (/forget).
	DeleteReporter(ctx context.Context, reporterHash string) (int, error)

	CreateDelisting(ctx context.Context, d Delisting) (Delisting, error)
	ListDelistings(ctx context.Context, status string) ([]Delisting, error)
	// DecideDelisting moves a pending request to approved/rejected.
	// It returns ErrNotFound if no pending request has that ID.
	DecideDelisting(ctx context.Context, id int64, status, note string, at time.Time) (Delisting, error)

	Ping(ctx context.Context) error
	Close()
}
