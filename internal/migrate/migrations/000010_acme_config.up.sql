-- Runtime-mutable ACME / TLS configuration managed from the admin UI.

CREATE TABLE IF NOT EXISTS acme_config (
    id                  SMALLINT PRIMARY KEY DEFAULT 1,
    email               TEXT NOT NULL DEFAULT '',
    directory           VARCHAR(16) NOT NULL DEFAULT 'staging',
    dns_provider        VARCHAR(32) NOT NULL DEFAULT 'cloudflare',
    dns_credentials_enc TEXT NOT NULL DEFAULT '',
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT acme_config_singleton CHECK (id = 1),
    CONSTRAINT acme_config_directory CHECK (directory IN ('staging', 'production'))
);

INSERT INTO acme_config (id) VALUES (1)
ON CONFLICT (id) DO NOTHING;

CREATE TABLE IF NOT EXISTS acme_domains (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    domain     TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE OR REPLACE FUNCTION update_acme_config_timestamp()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS acme_config_updated_at ON acme_config;
CREATE TRIGGER acme_config_updated_at
    BEFORE UPDATE ON acme_config
    FOR EACH ROW
    EXECUTE FUNCTION update_acme_config_timestamp();
