package store

import (
	"context"
	_ "embed"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/guru-bharadwaj20/satark/apps/feed-sync/internal/feeds"
)

//go:embed schema.sql
var schema string

// Postgres keeps the full history of feed entries (first/last seen, active)
// for reporting and for rebuilding Valkey if it is lost.
type Postgres struct{ Pool *pgxpool.Pool }

// Migrate applies the schema idempotently.
func (p *Postgres) Migrate(ctx context.Context) error {
	_, err := p.Pool.Exec(ctx, schema)
	return err
}

// Name implements syncer.Sink.
func (*Postgres) Name() string { return "postgres" }

// Store implements syncer.Sink. Entries present in the snapshot are upserted
// as active; entries of the same feed missing from it are deactivated.
func (p *Postgres) Store(ctx context.Context, feed string, entries []feeds.Entry, at time.Time) error {
	tx, err := p.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.WithoutCancel(ctx)) //nolint:errcheck // no-op after commit

	if _, err := tx.Exec(ctx, `CREATE TEMP TABLE feed_snapshot (url text PRIMARY KEY, host text NOT NULL) ON COMMIT DROP`); err != nil {
		return fmt.Errorf("temp table: %w", err)
	}
	rows := make([][]any, 0, len(entries))
	for _, e := range entries {
		rows = append(rows, []any{e.URL, e.Host})
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"feed_snapshot"}, []string{"url", "host"}, pgx.CopyFromRows(rows)); err != nil {
		return fmt.Errorf("copy: %w", err)
	}

	var added int
	err = tx.QueryRow(ctx, `
		WITH up AS (
			INSERT INTO threat_feed_entries (feed, url, host, first_seen, last_seen, active)
			SELECT $1, url, host, $2, $2, true FROM feed_snapshot
			ON CONFLICT (feed, url) DO UPDATE
				SET last_seen = EXCLUDED.last_seen, host = EXCLUDED.host, active = true
			RETURNING (xmax = 0) AS inserted
		)
		SELECT count(*) FILTER (WHERE inserted) FROM up`, feed, at).Scan(&added)
	if err != nil {
		return fmt.Errorf("upsert: %w", err)
	}
	tag, err := tx.Exec(ctx, `
		UPDATE threat_feed_entries SET active = false
		WHERE feed = $1 AND active AND last_seen < $2`, feed, at)
	if err != nil {
		return fmt.Errorf("deactivate: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO threat_feed_syncs (feed, synced_at, entries, added, deactivated)
		VALUES ($1, $2, $3, $4, $5)`, feed, at, len(entries), added, tag.RowsAffected()); err != nil {
		return fmt.Errorf("record sync: %w", err)
	}
	return tx.Commit(ctx)
}
