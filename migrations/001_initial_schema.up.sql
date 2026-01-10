-- Migration initiale : Schéma de base de données Essensys
-- Migration depuis SQL Server (NHibernate) vers PostgreSQL

-- Table des index de données (référentiel)
CREATE TABLE es_data_index (
    id SERIAL PRIMARY KEY,
    index_key VARCHAR(50) NOT NULL UNIQUE,
    is_active BOOLEAN NOT NULL DEFAULT true,
    date_creation TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    date_modification TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_data_index_key ON es_data_index(index_key);
CREATE INDEX idx_data_index_active ON es_data_index(is_active);

-- Table des machines (boîtiers Essensys)
CREATE TABLE es_machine (
    id SERIAL PRIMARY KEY,
    no_serie VARCHAR(100) NOT NULL UNIQUE,
    version VARCHAR(50),
    pkey VARCHAR(256) NOT NULL,
    hashed_pkey VARCHAR(256),
    autorise_alarme BOOLEAN NOT NULL DEFAULT false,
    is_active BOOLEAN NOT NULL DEFAULT true,
    date_creation TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    date_modification TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_machine_no_serie ON es_machine(no_serie);
CREATE INDEX idx_machine_active ON es_machine(is_active);

-- Table des utilisateurs
CREATE TABLE es_user (
    id SERIAL PRIMARY KEY,
    mail VARCHAR(255) NOT NULL UNIQUE,
    password VARCHAR(256) NOT NULL, -- Hash SHA1 (legacy)
    nom VARCHAR(255) NOT NULL,
    prenom VARCHAR(255) NOT NULL,
    adr1 VARCHAR(255),
    adr2 VARCHAR(255),
    cp VARCHAR(10),
    ville VARCHAR(255),
    phone VARCHAR(50),
    question VARCHAR(255) NOT NULL,
    reponse VARCHAR(256) NOT NULL, -- Hash SHA1
    isvalid BOOLEAN NOT NULL DEFAULT false,
    send_infos BOOLEAN NOT NULL DEFAULT false,
    obsolete BOOLEAN NOT NULL DEFAULT false,
    date_creation TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    date_cloture TIMESTAMP,
    last_access TIMESTAMP,
    guid VARCHAR(100) UNIQUE, -- Pour validation de compte
    machine_id INTEGER REFERENCES es_machine(id) ON DELETE SET NULL
);

CREATE INDEX idx_user_mail ON es_user(mail);
CREATE INDEX idx_user_guid ON es_user(guid);
CREATE INDEX idx_user_machine ON es_user(machine_id);
CREATE INDEX idx_user_valid ON es_user(isvalid);
CREATE INDEX idx_user_obsolete ON es_user(obsolete);

-- Table des clés d'activation (cle_machine)
CREATE TABLE es_cle_machine (
    id SERIAL PRIMARY KEY,
    cle VARCHAR(100) NOT NULL UNIQUE,
    date_generation TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    date_activation TIMESTAMP,
    machine_id INTEGER REFERENCES es_machine(id) ON DELETE SET NULL
);

CREATE INDEX idx_cle_machine_cle ON es_cle_machine(cle);
CREATE INDEX idx_cle_machine_machine ON es_cle_machine(machine_id);

-- Table des téléphones
CREATE TABLE es_phone (
    id SERIAL PRIMARY KEY,
    phone VARCHAR(50) NOT NULL,
    nom VARCHAR(255),
    send_mail BOOLEAN NOT NULL DEFAULT false,
    alerte_alarme_sent BOOLEAN NOT NULL DEFAULT false,
    alerte_lv_sent BOOLEAN NOT NULL DEFAULT false,
    alerte_ll_sent BOOLEAN NOT NULL DEFAULT false,
    alerte_no_sync BOOLEAN NOT NULL DEFAULT false,
    date_creation TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    date_modification TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    user_id INTEGER REFERENCES es_user(id) ON DELETE CASCADE
);

CREATE INDEX idx_phone_user ON es_phone(user_id);

-- Table des SMS envoyés
CREATE TABLE es_sms_send (
    id SERIAL PRIMARY KEY,
    phone VARCHAR(50) NOT NULL,
    message TEXT NOT NULL,
    date_sent TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    user_id INTEGER REFERENCES es_user(id) ON DELETE SET NULL
);

CREATE INDEX idx_sms_send_user ON es_sms_send(user_id);
CREATE INDEX idx_sms_send_date ON es_sms_send(date_sent);

-- Table des actions
CREATE TABLE es_action (
    id SERIAL PRIMARY KEY,
    machine_id INTEGER NOT NULL REFERENCES es_machine(id) ON DELETE CASCADE,
    guid VARCHAR(100) NOT NULL UNIQUE,
    action_type VARCHAR(50) NOT NULL,
    action_info TEXT,
    is_done BOOLEAN NOT NULL DEFAULT false,
    date_creation TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_action_machine ON es_action(machine_id);
CREATE INDEX idx_action_guid ON es_action(guid);
CREATE INDEX idx_action_done ON es_action(is_done);
CREATE INDEX idx_action_type ON es_action(action_type);
CREATE INDEX idx_action_date_creation ON es_action(date_creation);

-- Table des index d'actions (paramètres d'une action)
CREATE TABLE es_action_index (
    id SERIAL PRIMARY KEY,
    action_id INTEGER NOT NULL REFERENCES es_action(id) ON DELETE CASCADE,
    index_id INTEGER NOT NULL REFERENCES es_data_index(id) ON DELETE CASCADE,
    value VARCHAR(255) NOT NULL,
    UNIQUE(action_id, index_id)
);

CREATE INDEX idx_action_index_action ON es_action_index(action_id);
CREATE INDEX idx_action_index_index ON es_action_index(index_id);

-- Table des états (snapshots de l'état d'une machine)
CREATE TABLE es_state (
    id SERIAL PRIMARY KEY,
    machine_id INTEGER NOT NULL REFERENCES es_machine(id) ON DELETE CASCADE,
    version VARCHAR(50),
    completed BOOLEAN NOT NULL DEFAULT false,
    state_date TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_state_machine ON es_state(machine_id);
CREATE INDEX idx_state_date ON es_state(state_date);
CREATE INDEX idx_state_completed ON es_state(completed);

-- Table des index d'états (valeurs des index dans un état)
CREATE TABLE es_state_index (
    id SERIAL PRIMARY KEY,
    state_id INTEGER NOT NULL REFERENCES es_state(id) ON DELETE CASCADE,
    index_id INTEGER NOT NULL REFERENCES es_data_index(id) ON DELETE CASCADE,
    value VARCHAR(255) NOT NULL,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(state_id, index_id)
);

CREATE INDEX idx_state_index_state ON es_state_index(state_id);
CREATE INDEX idx_state_index_index ON es_state_index(index_id);
CREATE INDEX idx_state_index_updated ON es_state_index(updated_at);

-- Table des versions de firmware
CREATE TABLE es_version (
    id SERIAL PRIMARY KEY,
    descriptif TEXT,
    size INTEGER NOT NULL,
    filename VARCHAR(255) NOT NULL
);

CREATE INDEX idx_version_filename ON es_version(filename);

-- Table de suivi des versions par machine
CREATE TABLE es_version_machine (
    id SERIAL PRIMARY KEY,
    machine_id INTEGER NOT NULL REFERENCES es_machine(id) ON DELETE CASCADE,
    version VARCHAR(50) NOT NULL,
    is_ok BOOLEAN NOT NULL DEFAULT false,
    last_index_call INTEGER NOT NULL DEFAULT 0,
    date_action TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(machine_id, version)
);

CREATE INDEX idx_version_machine_machine ON es_version_machine(machine_id);
CREATE INDEX idx_version_machine_version ON es_version_machine(version);
CREATE INDEX idx_version_machine_ok ON es_version_machine(is_ok);

-- Insertion des index de données de base (exemples courants)
-- Ces index correspondent aux indices utilisés dans le système (605-622 pour lumières, etc.)
INSERT INTO es_data_index (index_key, is_active) VALUES
    ('1', true), ('2', true), ('349', true), ('350', true), ('351', true), ('352', true),
    ('363', true), ('407', true), ('425', true), ('426', true), ('590', true), ('605', true),
    ('606', true), ('607', true), ('608', true), ('609', true), ('610', true), ('611', true),
    ('612', true), ('613', true), ('614', true), ('615', true), ('616', true), ('617', true),
    ('618', true), ('619', true), ('620', true), ('621', true), ('622', true), ('920', true)
ON CONFLICT (index_key) DO NOTHING;


