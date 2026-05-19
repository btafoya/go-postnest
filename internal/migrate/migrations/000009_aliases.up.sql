-- User aliases and per-domain catch-all routing

CREATE TABLE IF NOT EXISTS aliases (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    domain_id UUID NOT NULL REFERENCES domains(id) ON DELETE CASCADE,
    local_part VARCHAR(64) NOT NULL,
    created_at TIMESTAMPTZ DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS aliases_domain_local_idx ON aliases(domain_id, local_part);

CREATE TABLE IF NOT EXISTS alias_targets (
    alias_id UUID NOT NULL REFERENCES aliases(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    PRIMARY KEY (alias_id, user_id)
);
CREATE INDEX IF NOT EXISTS alias_targets_user_id_idx ON alias_targets(user_id);

ALTER TABLE domains ADD COLUMN IF NOT EXISTS catchall_user_id UUID REFERENCES users(id) ON DELETE SET NULL;
