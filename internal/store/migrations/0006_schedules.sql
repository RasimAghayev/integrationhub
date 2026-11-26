CREATE TABLE IF NOT EXISTS schedules (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name           TEXT NOT NULL,
    integration_id UUID NOT NULL REFERENCES integrations(id) ON DELETE CASCADE,
    endpoint       TEXT NOT NULL,
    cron           TEXT NOT NULL,
    enabled        BOOLEAN NOT NULL DEFAULT TRUE,
    next_run       TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_run       TIMESTAMPTZ,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_schedules_due ON schedules (enabled, next_run);