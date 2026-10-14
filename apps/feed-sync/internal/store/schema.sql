-- Threat-feed tables in the satark database (cluster satark-pg).
-- Applied idempotently by feed-sync on start-up.

CREATE TABLE IF NOT EXISTS threat_feed_entries (
    feed        text        NOT NULL,
    url         text        NOT NULL,
    host        text        NOT NULL,
    first_seen  timestamptz NOT NULL,
    last_seen   timestamptz NOT NULL,
    active      boolean     NOT NULL DEFAULT true,
    PRIMARY KEY (feed, url)
);

CREATE INDEX IF NOT EXISTS threat_feed_entries_host_idx
    ON threat_feed_entries (host) WHERE active;

CREATE TABLE IF NOT EXISTS threat_feed_syncs (
    id          bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    feed        text        NOT NULL,
    synced_at   timestamptz NOT NULL,
    entries     integer     NOT NULL,
    added       integer     NOT NULL,
    deactivated integer     NOT NULL
);

CREATE INDEX IF NOT EXISTS threat_feed_syncs_feed_time_idx
    ON threat_feed_syncs (feed, synced_at DESC);
