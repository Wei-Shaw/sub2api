-- Ensure usage_logs cache token columns use the underscored names expected by code.
-- Backfill from legacy column names if they exist.

ALTER TABLE usage_logs
    ADD COLUMN IF NOT EXISTS cache_creation_5m_tokens INT NOT NULL DEFAULT 0;

ALTER TABLE usage_logs
    ADD COLUMN IF NOT EXISTS cache_creation_1h_tokens INT NOT NULL DEFAULT 0;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_schema = 'public'
          AND table_name = 'usage_logs'
          AND column_name = 'cache_creation5m_tokens'
    ) THEN
        UPDATE usage_logs
        SET cache_creation_5m_tokens = cache_creation5m_tokens
        WHERE cache_creation_5m_tokens = 0
          AND cache_creation5m_tokens <> 0;
    END IF;

    IF EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_schema = 'public'
          AND table_name = 'usage_logs'
          AND column_name = 'cache_creation1h_tokens'
    ) THEN
        UPDATE usage_logs
        SET cache_creation_1h_tokens = cache_creation1h_tokens
        WHERE cache_creation_1h_tokens = 0
          AND cache_creation1h_tokens <> 0;
    END IF;
END $$;
