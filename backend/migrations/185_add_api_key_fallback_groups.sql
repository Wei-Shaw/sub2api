-- Preserve the ordered standby group list for each API key.  PostgreSQL's
-- JSONB type matches Ent's []int64 JSON field and makes existing keys default
-- safely to an empty fallback list.
ALTER TABLE api_keys
    ADD COLUMN IF NOT EXISTS fallback_group_ids JSONB NOT NULL DEFAULT '[]'::jsonb;
