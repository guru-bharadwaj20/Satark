// Package store persists scan results to PostgreSQL (satark-pg, schema in
// apps/gateway-api/migrations).
//
// Only HMAC digests of inputs are written, never the raw text (see
// internal/hashing).
//
// Wiring (gateway-api main), requires github.com/jackc/pgx/v5 in go.mod:
//
//	pool, err := store.Connect(ctx, os.Getenv("DATABASE_URL"))
//	defer pool.Close()
//	st := store.New(pool, hasher, logger)
//	// in the scan handler, after scoring:
//	id, err := st.SaveScan(ctx, store.Scan{Kind: store.KindText, Input: req.Text, ...})
//
// A failed write must not fail the user's request: log it and return the
// verdict (SaveScan already bounds itself with WriteTimeout). /readyz can call
// st.Ping.
package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/guru-bharadwaj20/satark/apps/gateway-api/internal/hashing"
)

// WriteTimeout bounds every database call made by the store.
const WriteTimeout = 2 * time.Second

// ScanKind matches the scans.kind CHECK constraint.
type ScanKind string

const (
	KindText ScanKind = "text"
	KindQR   ScanKind = "qr"
	KindURL  ScanKind = "url"
)

// Verdict matches the scans.verdict CHECK constraint.
type Verdict string

const (
	VerdictLikelyScam Verdict = "likely_scam"
	VerdictSuspicious Verdict = "suspicious"
	VerdictNoRedFlags Verdict = "no_red_flags"
)

// Scan is one handled scan request. Input is hashed before it is written.
type Scan struct {
	Kind       ScanKind
	Input      string
	Channel    string // api | telegram | pwa; defaults to api
	Lang       string // en | hi | kn; defaults to en
	RiskScore  int
	Verdict    Verdict
	Reasons    []string // reason codes, e.g. "urgency", "personal_number"
	AnalyzerMS int
}

// DB is the subset of *pgxpool.Pool the store uses (eases testing).
type DB interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Store writes scan results.
type Store struct {
	db     DB
	hasher *hashing.Hasher
	log    *slog.Logger
}

var ErrInvalidScan = errors.New("store: invalid scan")

// Connect opens a pgx pool and verifies connectivity.
func Connect(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("store: parse dsn: %w", err)
	}
	cfg.MaxConns = 10
	cfg.MaxConnIdleTime = 5 * time.Minute
	cfg.HealthCheckPeriod = 30 * time.Second
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("store: connect: %w", err)
	}
	pctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(pctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("store: ping: %w", err)
	}
	return pool, nil
}

// New returns a Store. log may be nil.
func New(db DB, h *hashing.Hasher, log *slog.Logger) *Store {
	if log == nil {
		log = slog.Default()
	}
	return &Store{db: db, hasher: h, log: log}
}

const insertScan = `INSERT INTO scans
	(kind, input_hash, channel, lang, risk_score, verdict, reasons, analyzer_ms)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	RETURNING id::text`

// SaveScan hashes the input and inserts a row into scans, returning its id.
func (s *Store) SaveScan(ctx context.Context, sc Scan) (string, error) {
	if err := validate(&sc); err != nil {
		return "", err
	}
	digest, err := s.hasher.Input(hashing.Kind(sc.Kind), sc.Input)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalidScan, err)
	}
	reasons, err := json.Marshal(sc.Reasons)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, WriteTimeout)
	defer cancel()

	var id string
	start := time.Now()
	err = s.db.QueryRow(ctx, insertScan,
		string(sc.Kind), digest, sc.Channel, sc.Lang, sc.RiskScore,
		string(sc.Verdict), reasons, sc.AnalyzerMS,
	).Scan(&id)
	if err != nil {
		s.log.ErrorContext(ctx, "save scan failed", "kind", sc.Kind, "err", err)
		return "", fmt.Errorf("store: insert scan: %w", err)
	}
	s.log.DebugContext(ctx, "scan saved", "id", id, "kind", sc.Kind,
		"verdict", sc.Verdict, "db_ms", time.Since(start).Milliseconds())
	return id, nil
}

// SaveFeedback records a user rating (correct | wrong | unsure) for a scan.
func (s *Store) SaveFeedback(ctx context.Context, scanID, rating string) error {
	switch rating {
	case "correct", "wrong", "unsure":
	default:
		return fmt.Errorf("%w: rating %q", ErrInvalidScan, rating)
	}
	ctx, cancel := context.WithTimeout(ctx, WriteTimeout)
	defer cancel()
	_, err := s.db.Exec(ctx,
		`INSERT INTO feedback (scan_id, rating) VALUES ($1::uuid, $2)`, scanID, rating)
	if err != nil {
		return fmt.Errorf("store: insert feedback: %w", err)
	}
	return nil
}

// Ping checks the database for readiness probes.
func (s *Store) Ping(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, WriteTimeout)
	defer cancel()
	var one int
	return s.db.QueryRow(ctx, "SELECT 1").Scan(&one)
}

func validate(sc *Scan) error {
	switch sc.Kind {
	case KindText, KindQR, KindURL:
	default:
		return fmt.Errorf("%w: kind %q", ErrInvalidScan, sc.Kind)
	}
	switch sc.Verdict {
	case VerdictLikelyScam, VerdictSuspicious, VerdictNoRedFlags:
	default:
		return fmt.Errorf("%w: verdict %q", ErrInvalidScan, sc.Verdict)
	}
	if sc.RiskScore < 0 || sc.RiskScore > 100 {
		return fmt.Errorf("%w: risk score %d", ErrInvalidScan, sc.RiskScore)
	}
	if sc.AnalyzerMS < 0 {
		return fmt.Errorf("%w: negative analyzer time", ErrInvalidScan)
	}
	if sc.Channel == "" {
		sc.Channel = "api"
	}
	if sc.Lang == "" {
		sc.Lang = "en"
	}
	if sc.Reasons == nil {
		sc.Reasons = []string{}
	}
	return nil
}
