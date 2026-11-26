CREATE TABLE IF NOT EXISTS integrations (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name          TEXT NOT NULL,
    description   TEXT NOT NULL DEFAULT '',
    type          TEXT NOT NULL,
    status        TEXT NOT NULL DEFAULT 'active',
    connection_id TEXT NOT NULL REFERENCES connections(id) ON DELETE CASCADE,
    endpoints     JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_integrations_connection ON integrations(connection_id);