CREATE TABLE IF NOT EXISTS executions (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    integration_id UUID NOT NULL REFERENCES integrations(id) ON DELETE CASCADE,
    endpoint       TEXT NOT NULL,
    status         TEXT NOT NULL DEFAULT 'queued',
    request_body   TEXT NOT NULL DEFAULT '',
    response_body  TEXT NOT NULL DEFAULT '',
    status_code    INTEGER NOT NULL DEFAULT 0,
    attempts       INTEGER NOT NULL DEFAULT 0,
    error          TEXT NOT NULL DEFAULT '',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    started_at     TIMESTAMPTZ,
    finished_at    TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_executions_integration
    ON executions(integration_id, created_at DESC);

-- Partial index keeps the worker's claim query fast as history grows.
CREATE INDEX IF NOT EXISTS idx_executions_pending
    ON executions(created_at)
    WHERE status IN ('queued', 'running');