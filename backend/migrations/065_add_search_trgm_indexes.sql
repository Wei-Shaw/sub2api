-- Improve admin fuzzy-search performance on large datasets.
-- Best effort:
-- 1) try enabling pg_trgm
-- 2) only create trigram indexes when extension is available
DO $$
BEGIN
    BEGIN
        CREATE EXTENSION IF NOT EXISTS pg_trgm;
    EXCEPTION
        WHEN OTHERS THEN
            RAISE NOTICE 'pg_trgm extension not created: %', SQLERRM;
    END;

    IF EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'pg_trgm') THEN
        EXECUTE 'CREATE INDEX IF NOT EXISTS idx_users_email_trgm
                 ON users USING gin (email gin_trgm_ops)';
        EXECUTE 'CREATE INDEX IF NOT EXISTS idx_users_username_trgm
                 ON users USING gin (username gin_trgm_ops)';
        EXECUTE 'CREATE INDEX IF NOT EXISTS idx_users_notes_trgm
                 ON users USING gin (notes gin_trgm_ops)';

        EXECUTE 'CREATE INDEX IF NOT EXISTS idx_accounts_name_trgm
                 ON accounts USING gin (name gin_trgm_ops)';

        EXECUTE 'CREATE INDEX IF NOT EXISTS idx_api_keys_key_trgm
                 ON api_keys USING gin ("key" gin_trgm_ops)';
        EXECUTE 'CREATE INDEX IF NOT EXISTS idx_api_keys_name_trgm
                 ON api_keys USING gin (name gin_trgm_ops)';
    ELSE
        RAISE NOTICE 'skip trigram indexes because pg_trgm is unavailable';
    END IF;
END
$$;
