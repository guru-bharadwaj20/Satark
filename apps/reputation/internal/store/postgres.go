package store

import (
	"context"
	_ "embed"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed schema.sql
var schema string

// Postgres is the production Store backed by the satark-pg cluster.
type Postgres struct{ pool *pgxpool.Pool }

// NewPostgres connects to dsn and applies the (idempotent) schema.
func NewPostgres(ctx context.Context, dsn string) (*Postgres, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, err
	}
	if _, err := pool.Exec(ctx, schema); err != nil {
		pool.Close()
		return nil, err
	}
	return &Postgres{pool: pool}, nil
}

func (p *Postgres) AddReport(ctx context.Context, r Report) (bool, error) {
	tag, err := p.pool.Exec(ctx, `
		INSERT INTO reputation_reports (kind, subject_hash, reporter_hash, category, created_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (kind, subject_hash, reporter_hash) DO NOTHING`,
		r.Kind, r.SubjectHash, r.ReporterHash, r.Category, r.CreatedAt)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

func (p *Postgres) Summary(ctx context.Context, kind, subject string) (Summary, error) {
	s := Summary{Categories: map[string]int{}}
	rows, err := p.pool.Query(ctx, `
		SELECT category, count(*), min(created_at), max(created_at)
		FROM reputation_reports
		WHERE kind = $1 AND subject_hash = $2
		GROUP BY category`, kind, subject)
	if err != nil {
		return s, err
	}
	defer rows.Close()
	for rows.Next() {
		var cat string
		var n int
		var first, last = s.FirstSeen, s.LastSeen
		if err := rows.Scan(&cat, &n, &first, &last); err != nil {
			return s, err
		}
		s.Categories[cat] = n
		s.DistinctReporters += n
		if s.FirstSeen.IsZero() || first.Before(s.FirstSeen) {
			s.FirstSeen = first
		}
		if last.After(s.LastSeen) {
			s.LastSeen = last
		}
	}
	return s, rows.Err()
}

func (p *Postgres) Ping(ctx context.Context) error { return p.pool.Ping(ctx) }
func (p *Postgres) Close()                         { p.pool.Close() }
