CREATE TABLE IF NOT EXISTS connections (
    id          TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    name        TEXT NOT NULL,
    type        TEXT NOT NULL,
    auth_type   TEXT NOT NULL DEFAULT 'none',
    config      JSONB NOT NULL DEFAULT '{}'::jsonb,
    secret_ref  TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_connections_name ON connections (name);