-- Add hashed API key storage columns and relax legacy key constraints
ALTER TABLE api_keys
    ALTER COLUMN key DROP NOT NULL,
    ALTER COLUMN key TYPE VARCHAR(128);

ALTER TABLE api_keys
    ADD COLUMN IF NOT EXISTS key_hash VARCHAR(64),
    ADD COLUMN IF NOT EXISTS key_last4 VARCHAR(4);

CREATE UNIQUE INDEX IF NOT EXISTS idx_api_keys_key_hash_unique ON api_keys(key_hash);
