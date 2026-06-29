-- Trusted devices LAN (OpenSpec 2026-06.013)

CREATE TABLE IF NOT EXISTS trusted_devices (
    id SERIAL PRIMARY KEY,
    lan_user_id INTEGER NOT NULL REFERENCES lan_users(id) ON DELETE CASCADE,
    mac_address VARCHAR(17) NOT NULL,
    device_label VARCHAR(255) NOT NULL DEFAULT '',
    trust_mode VARCHAR(32) NOT NULL
        CHECK (trust_mode IN ('temporary', 'permanent')),
    expires_at TIMESTAMPTZ NULL,
    created_by_user_id INTEGER NULL REFERENCES lan_users(id) ON DELETE SET NULL,
    approved_by_admin_user_id INTEGER NULL REFERENCES lan_users(id) ON DELETE SET NULL,
    last_seen_at TIMESTAMPTZ NULL,
    revoked_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_trusted_devices_active_user_mac
    ON trusted_devices (lan_user_id, mac_address)
    WHERE revoked_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_trusted_devices_mac_active
    ON trusted_devices (mac_address)
    WHERE revoked_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_trusted_devices_user_active
    ON trusted_devices (lan_user_id)
    WHERE revoked_at IS NULL;
