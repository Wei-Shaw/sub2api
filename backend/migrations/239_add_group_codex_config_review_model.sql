ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS codex_config_review_model VARCHAR(200) NOT NULL DEFAULT '';

COMMENT ON COLUMN groups.codex_config_review_model IS
    'Codex config review model for this group; empty uses the explicit preferred model, then the hardcoded platform default, never the catalog first model';
