ALTER TABLE composite_model_routes
    ADD COLUMN IF NOT EXISTS fallback_targets jsonb NOT NULL DEFAULT '[]'::jsonb;
