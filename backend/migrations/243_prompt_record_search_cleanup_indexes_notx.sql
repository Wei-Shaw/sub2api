CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_prompt_records_expires_at_id
    ON prompt_records (expires_at, id) WHERE expires_at IS NOT NULL;

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_prompt_records_model_trgm
    ON prompt_records USING gin (model gin_trgm_ops);
