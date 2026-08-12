ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS long_context_pricing_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS model_pricing JSONB;

COMMENT ON COLUMN groups.long_context_pricing_enabled IS
    'Whether token pricing selects long-context tiers; false uses the lowest tier only';
COMMENT ON COLUMN groups.model_pricing IS
    'Per-model group pricing overrides channel and built-in model pricing';
