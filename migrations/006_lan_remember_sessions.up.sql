-- Sessions LAN mémorisées « Se souvenir de moi » : 30 jours fixes, conservées au
-- redémarrage de la gateway (session-30-jours-2026-11-001, #14).
-- Seule l'empreinte SHA-256 de l'identifiant de session est stockée, jamais l'identifiant.

CREATE TABLE IF NOT EXISTS lan_remember_sessions (
    id SERIAL PRIMARY KEY,
    id_hash CHAR(64) NOT NULL,
    lan_user_id INTEGER NOT NULL REFERENCES lan_users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    user_agent VARCHAR(255) NOT NULL DEFAULT ''
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_lan_remember_sessions_id_hash
    ON lan_remember_sessions (id_hash);

CREATE INDEX IF NOT EXISTS idx_lan_remember_sessions_user
    ON lan_remember_sessions (lan_user_id);

CREATE INDEX IF NOT EXISTS idx_lan_remember_sessions_expires
    ON lan_remember_sessions (expires_at);
