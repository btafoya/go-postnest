-- Add runtime-enabled flag to ACME configuration.

ALTER TABLE acme_config ADD COLUMN IF NOT EXISTS enabled BOOLEAN NOT NULL DEFAULT false;
