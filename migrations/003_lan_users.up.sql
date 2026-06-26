-- IAM LAN (OpenSpec 2026-06.017) — comptes locaux gateway, distinct de es_user legacy

CREATE TABLE IF NOT EXISTS lan_users (
    id SERIAL PRIMARY KEY,
    email VARCHAR(255) NOT NULL UNIQUE,
    password_hash VARCHAR(255) NOT NULL,
    password_algo VARCHAR(32) NOT NULL DEFAULT 'bcrypt',
    role VARCHAR(32) NOT NULL DEFAULT 'lan_user'
        CHECK (role IN ('lan_admin', 'lan_user', 'lan_guest')),
    display_name VARCHAR(255) NOT NULL DEFAULT '',
    disabled_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_login_at TIMESTAMPTZ NULL
);

CREATE INDEX IF NOT EXISTS idx_lan_users_email ON lan_users(email);
CREATE INDEX IF NOT EXISTS idx_lan_users_role ON lan_users(role);
CREATE INDEX IF NOT EXISTS idx_lan_users_disabled ON lan_users(disabled_at);
