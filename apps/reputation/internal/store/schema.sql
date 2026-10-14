-- reputation schema (idempotent). Subjects and reporters are HMAC hashes.
CREATE TABLE IF NOT EXISTS reputation_reports (
    id            BIGSERIAL PRIMARY KEY,
    kind          TEXT        NOT NULL CHECK (kind IN ('phone', 'upi', 'domain')),
    subject_hash  TEXT        NOT NULL,
    reporter_hash TEXT        NOT NULL,
    category      TEXT        NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (kind, subject_hash, reporter_hash)
);
CREATE INDEX IF NOT EXISTS reputation_reports_subject_idx ON reputation_reports (kind, subject_hash);
CREATE INDEX IF NOT EXISTS reputation_reports_created_idx ON reputation_reports (created_at);
CREATE INDEX IF NOT EXISTS reputation_reports_reporter_idx ON reputation_reports (reporter_hash);

-- Delisting requests (SOP-07). contact is optional and purged by retention.
CREATE TABLE IF NOT EXISTS reputation_delistings (
    id             BIGSERIAL PRIMARY KEY,
    kind           TEXT        NOT NULL,
    subject_hash   TEXT        NOT NULL,
    subject_masked TEXT        NOT NULL,
    reason         TEXT        NOT NULL,
    contact        TEXT        NOT NULL DEFAULT '',
    status         TEXT        NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'approved', 'rejected')),
    review_note    TEXT        NOT NULL DEFAULT '',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    decided_at     TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS reputation_delistings_status_idx ON reputation_delistings (status, created_at);
