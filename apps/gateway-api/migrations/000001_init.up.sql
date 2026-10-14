-- Satark core schema.
-- Privacy: no raw message text, phone number or UPI ID is stored. Inputs and
-- entities are stored as HMAC-SHA256 digests (see internal/hashing) plus, for
-- entities, a masked display form such as 98XXXXXX21 or xy***@examplebank.

BEGIN;

CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- One row per scan request handled by gateway-api.
CREATE TABLE scans (
    id             uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    kind           text        NOT NULL CHECK (kind IN ('text', 'qr', 'url')),
    input_hash     bytea       NOT NULL CHECK (octet_length(input_hash) = 32),
    channel        text        NOT NULL DEFAULT 'api'
                               CHECK (channel IN ('api', 'telegram', 'pwa')),
    lang           text        NOT NULL DEFAULT 'en' CHECK (lang IN ('en', 'hi', 'kn')),
    risk_score     smallint    NOT NULL CHECK (risk_score BETWEEN 0 AND 100),
    verdict        text        NOT NULL
                               CHECK (verdict IN ('likely_scam', 'suspicious', 'no_red_flags')),
    reasons        jsonb       NOT NULL DEFAULT '[]'::jsonb,
    analyzer_ms    integer     CHECK (analyzer_ms >= 0),
    created_at     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX scans_input_hash_idx ON scans (input_hash, created_at DESC);
CREATE INDEX scans_created_at_idx ON scans (created_at);

-- Cached link intelligence, keyed by the normalized URL's hash.
CREATE TABLE url_intel (
    url_hash         bytea       PRIMARY KEY CHECK (octet_length(url_hash) = 32),
    final_domain     text        NOT NULL,
    redirect_hops    smallint    NOT NULL DEFAULT 0 CHECK (redirect_hops >= 0),
    domain_age_days  integer,
    tls_age_days     integer,
    lookalike_of     text,
    feed_hits        text[]      NOT NULL DEFAULT '{}',
    sandbox_flags    jsonb       NOT NULL DEFAULT '{}'::jsonb,
    risk_score       smallint    NOT NULL CHECK (risk_score BETWEEN 0 AND 100),
    first_seen_at    timestamptz NOT NULL DEFAULT now(),
    checked_at       timestamptz NOT NULL DEFAULT now(),
    expires_at       timestamptz NOT NULL
);
CREATE INDEX url_intel_final_domain_idx ON url_intel (final_domain);
CREATE INDEX url_intel_expires_at_idx ON url_intel (expires_at);

-- Reportable entities: phone numbers, UPI IDs and domains.
CREATE TABLE entities (
    id            bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    kind          text        NOT NULL CHECK (kind IN ('phone', 'upi', 'domain')),
    value_hash    bytea       NOT NULL CHECK (octet_length(value_hash) = 32),
    masked        text        NOT NULL,
    report_count  integer     NOT NULL DEFAULT 0 CHECK (report_count >= 0),
    first_seen_at timestamptz NOT NULL DEFAULT now(),
    last_seen_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE (kind, value_hash)
);

-- Community reports. reporter_hash is an HMAC of the channel user id so the
-- same person reporting twice counts once ("independent reports").
CREATE TABLE reports (
    id             uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    entity_id      bigint      NOT NULL REFERENCES entities (id) ON DELETE CASCADE,
    scan_id        uuid        REFERENCES scans (id) ON DELETE SET NULL,
    reporter_hash  bytea       NOT NULL CHECK (octet_length(reporter_hash) = 32),
    category       text        NOT NULL
                               CHECK (category IN ('kyc', 'utility', 'otp', 'lottery', 'task_job',
                                                   'remote_access', 'upi_collect', 'other')),
    created_at     timestamptz NOT NULL DEFAULT now(),
    UNIQUE (entity_id, reporter_hash)
);
CREATE INDEX reports_entity_created_idx ON reports (entity_id, created_at DESC);

-- User feedback on a verdict ("Was this helpful?").
CREATE TABLE feedback (
    id          bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    scan_id     uuid        NOT NULL REFERENCES scans (id) ON DELETE CASCADE,
    rating      text        NOT NULL CHECK (rating IN ('correct', 'wrong', 'unsure')),
    comment     text        CHECK (char_length(comment) <= 500),
    created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX feedback_scan_id_idx ON feedback (scan_id);

COMMIT;
