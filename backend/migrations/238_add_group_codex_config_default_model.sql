ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS codex_config_default_model VARCHAR(200) NOT NULL DEFAULT '';

COMMENT ON COLUMN groups.codex_config_default_model IS
    'Codex config preferred model for this group; empty uses the platform default';
