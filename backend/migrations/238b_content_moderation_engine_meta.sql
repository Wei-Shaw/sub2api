-- Audit provenance only; old rows and old application writes remain nullable.
ALTER TABLE content_moderation_logs ADD COLUMN IF NOT EXISTS engine_meta JSONB;
