package store

import (
	"context"
	_ "embed"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
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

func (p *Postgres) DeleteSubject(ctx context.Context, kind, subject string) (int, error) {
	tag, err := p.pool.Exec(ctx,
		`DELETE FROM reputation_reports WHERE kind = $1 AND subject_hash = $2`, kind, subject)
	return int(tag.RowsAffected()), err
}

func (p *Postgres) DeleteReporter(ctx context.Context, reporter string) (int, error) {
	tag, err := p.pool.Exec(ctx, `DELETE FROM reputation_reports WHERE reporter_hash = $1`, reporter)
	return int(tag.RowsAffected()), err
}

func (p *Postgres) Purge(ctx context.Context, pol RetentionPolicy) (PurgeResult, error) {
	var res PurgeResult
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return res, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op after commit
	steps := []struct {
		sql string
		arg time.Time
		n   *int
	}{
		{`DELETE FROM reputation_reports WHERE created_at < $1`, pol.ReportsBefore, &res.Reports},
		{`DELETE FROM reputation_delistings WHERE decided_at < $1`, pol.DecidedDelistsBefore, &res.Delistings},
		{`UPDATE reputation_delistings SET contact = '' WHERE decided_at < $1 AND contact <> ''`, pol.ContactsBefore, &res.ContactsCleared},
	}
	for _, s := range steps {
		tag, err := tx.Exec(ctx, s.sql, s.arg)
		if err != nil {
			return res, err
		}
		*s.n = int(tag.RowsAffected())
	}
	return res, tx.Commit(ctx)
}

func (p *Postgres) Trends(ctx context.Context, from, to time.Time) ([]TrendRow, error) {
	rows, err := p.pool.Query(ctx, `
		SELECT kind, category, count(*), count(DISTINCT reporter_hash), count(DISTINCT subject_hash)
		FROM reputation_reports
		WHERE created_at >= $1 AND created_at < $2
		GROUP BY kind, category`, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TrendRow
	for rows.Next() {
		var r TrendRow
		if err := rows.Scan(&r.Kind, &r.Category, &r.Reports, &r.Reporters, &r.Subjects); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

const delistCols = `id, kind, subject_hash, subject_masked, reason, contact, status, review_note, created_at, decided_at`

func scanDelisting(row pgx.Row) (Delisting, error) {
	var d Delisting
	err := row.Scan(&d.ID, &d.Kind, &d.SubjectHash, &d.SubjectMasked, &d.Reason,
		&d.Contact, &d.Status, &d.ReviewNote, &d.CreatedAt, &d.DecidedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return d, ErrNotFound
	}
	return d, err
}

func (p *Postgres) CreateDelisting(ctx context.Context, d Delisting) (Delisting, error) {
	return scanDelisting(p.pool.QueryRow(ctx, `
		INSERT INTO reputation_delistings (kind, subject_hash, subject_masked, reason, contact, status, created_at)
		VALUES ($1, $2, $3, $4, $5, 'pending', $6)
		RETURNING `+delistCols,
		d.Kind, d.SubjectHash, d.SubjectMasked, d.Reason, d.Contact, d.CreatedAt))
}

func (p *Postgres) ListDelistings(ctx context.Context, status string) ([]Delisting, error) {
	rows, err := p.pool.Query(ctx, `SELECT `+delistCols+` FROM reputation_delistings
		WHERE $1 = '' OR status = $1 ORDER BY created_at LIMIT 500`, status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Delisting
	for rows.Next() {
		d, err := scanDelisting(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (p *Postgres) DecideDelisting(ctx context.Context, id int64, status, note string, at time.Time) (Delisting, error) {
	return scanDelisting(p.pool.QueryRow(ctx, `
		UPDATE reputation_delistings SET status = $2, review_note = $3, decided_at = $4
		WHERE id = $1 AND status = 'pending'
		RETURNING `+delistCols, id, status, note, at))
}

func (p *Postgres) Ping(ctx context.Context) error { return p.pool.Ping(ctx) }
func (p *Postgres) Close()                         { p.pool.Close() }
