ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS codex_models_manifest_config JSONB NOT NULL DEFAULT '{}'::jsonb;

COMMENT ON COLUMN groups.codex_models_manifest_config IS
    'Pinned-accounts Codex models manifest config for OpenAI groups: {"enabled":bool,"account_ids":[int64],"fallback_to_scheduler":bool}; when enabled the Codex /models manifest is fetched only from the pinned accounts';
