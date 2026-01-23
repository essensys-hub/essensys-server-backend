-- Migration for client tracking table
-- Stores historical connection data every 10 minutes

CREATE TABLE es_client_tracking (
    id SERIAL PRIMARY KEY,
    client_id VARCHAR(100) NOT NULL,
    ip_address VARCHAR(50),
    version VARCHAR(50),
    raw_auth TEXT, -- Base64 credential
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_client_tracking_client ON es_client_tracking(client_id);
CREATE INDEX idx_client_tracking_date ON es_client_tracking(created_at);
