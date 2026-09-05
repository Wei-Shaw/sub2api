ALTER TABLE api_keys
    ADD COLUMN IF NOT EXISTS max_rate_multiplier DECIMAL(20,8);

COMMENT ON COLUMN api_keys.max_rate_multiplier IS
    'Maximum effective billing multiplier for this API key (NULL = unlimited)';
