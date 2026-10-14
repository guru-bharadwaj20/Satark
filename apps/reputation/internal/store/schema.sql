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
