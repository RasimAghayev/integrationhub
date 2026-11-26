CREATE TABLE IF NOT EXISTS refresh_tokens (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    family      UUID NOT NULL,
    subject     TEXT NOT NULL,
    roles       JSONB NOT NULL DEFAULT '[]'::jsonb,
    token_hash  TEXT NOT NULL UNIQUE,
    expires_at  TIMESTAMPTZ NOT NULL,
    revoked_at  TIMESTAMPTZ,
    replaced_by UUID,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_refresh_tokens_family ON refresh_tokens (family);
CREATE INDEX IF NOT EXISTS idx_refresh_tokens_subject ON refresh_tokens (subject);