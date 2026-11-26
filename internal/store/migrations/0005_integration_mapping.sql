ALTER TABLE integrations
    ADD COLUMN IF NOT EXISTS mappings JSONB NOT NULL DEFAULT '[]'::jsonb;

ALTER TABLE integrations
    ADD COLUMN IF NOT EXISTS transform JSONB NOT NULL DEFAULT '{}'::jsonb;