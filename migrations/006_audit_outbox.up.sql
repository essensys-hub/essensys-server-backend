-- Audit trail gateway (OpenSpec 2026-07-034 DEV 1)

CREATE TABLE IF NOT EXISTS armoire_audit_events (
    id              BIGSERIAL PRIMARY KEY,
    event_id        UUID NOT NULL UNIQUE,
    machine_id      INTEGER NOT NULL,
    gateway_id      VARCHAR(64),
    occurred_at     TIMESTAMPTZ NOT NULL,
    ingested_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    event_type      VARCHAR(32) NOT NULL,
    actor_type      VARCHAR(16) NOT NULL,
    actor_id        VARCHAR(128),
    actor_ip        INET,
    subject_type    VARCHAR(32) NOT NULL,
    subject_key     VARCHAR(128) NOT NULL,
    old_value       TEXT,
    new_value       TEXT,
    details         JSONB,
    source          VARCHAR(16) NOT NULL DEFAULT 'gateway',
    prev_hash       CHAR(64),
    event_hash      CHAR(64) NOT NULL,
    pending_sync    BOOLEAN NOT NULL DEFAULT FALSE
);

CREATE INDEX IF NOT EXISTS idx_audit_machine_time
    ON armoire_audit_events (machine_id, occurred_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_machine_subject
    ON armoire_audit_events (machine_id, subject_type, subject_key, occurred_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_event_type
    ON armoire_audit_events (machine_id, event_type, occurred_at DESC);

CREATE TABLE IF NOT EXISTS audit_outbox (
    event_id        UUID PRIMARY KEY,
    payload_json    JSONB NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    sync_status     VARCHAR(16) NOT NULL DEFAULT 'pending',
    sync_attempts   INTEGER NOT NULL DEFAULT 0,
    last_error      TEXT
);

CREATE INDEX IF NOT EXISTS idx_audit_outbox_pending
    ON audit_outbox (sync_status, created_at)
    WHERE sync_status = 'pending';

CREATE TABLE IF NOT EXISTS lan_audit_charter_acceptances (
    id               SERIAL PRIMARY KEY,
    lan_user_id      INTEGER NOT NULL REFERENCES lan_users(id) ON DELETE CASCADE,
    charter_version  VARCHAR(32) NOT NULL,
    accepted_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    ip_address       INET,
    UNIQUE (lan_user_id, charter_version)
);

CREATE INDEX IF NOT EXISTS idx_lan_audit_charter_user
    ON lan_audit_charter_acceptances (lan_user_id);
