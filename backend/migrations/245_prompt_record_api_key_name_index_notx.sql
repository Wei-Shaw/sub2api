CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_prompt_records_api_key_name_trgm
    ON prompt_records USING gin (api_key_name_snapshot gin_trgm_ops);
