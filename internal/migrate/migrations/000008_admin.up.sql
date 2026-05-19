-- Admin UI support: domain activation and system security settings

ALTER TABLE domains ADD COLUMN IF NOT EXISTS is_active BOOLEAN NOT NULL DEFAULT true;

CREATE TABLE IF NOT EXISTS system_settings (
    key VARCHAR(64) PRIMARY KEY NOT NULL,
    value TEXT NOT NULL,
    updated_at TIMESTAMPTZ DEFAULT now()
);

-- Default security settings
INSERT INTO system_settings (key, value) VALUES
    ('allow_insecure_auth', 'false'),
    ('require_strong_passwords', 'true'),
    ('session_timeout_minutes', '60'),
    ('rate_limit_requests_per_minute', '100')
ON CONFLICT (key) DO NOTHING;

CREATE OR REPLACE FUNCTION update_system_settings_timestamp()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS system_settings_updated_at ON system_settings;
CREATE TRIGGER system_settings_updated_at
    BEFORE UPDATE ON system_settings
    FOR EACH ROW
    EXECUTE FUNCTION update_system_settings_timestamp();
