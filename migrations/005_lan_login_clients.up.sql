-- Historique login LAN → client MAC (OpenSpec 2026-06.013)

CREATE TABLE IF NOT EXISTS lan_login_clients (
    id SERIAL PRIMARY KEY,
    lan_user_id INTEGER NOT NULL REFERENCES lan_users(id) ON DELETE CASCADE,
    mac_address VARCHAR(17) NOT NULL,
    source_ip VARCHAR(45) NOT NULL DEFAULT '',
    device_label VARCHAR(255) NOT NULL DEFAULT '',
    last_login_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_lan_login_clients_user_mac
    ON lan_login_clients (lan_user_id, mac_address);

CREATE INDEX IF NOT EXISTS idx_lan_login_clients_last_login
    ON lan_login_clients (last_login_at DESC);
