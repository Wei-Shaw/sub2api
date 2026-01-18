-- 兼容缺失 users.allowed_groups 的老库，确保 007 回填可执行。
DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM information_schema.tables
        WHERE table_schema = 'public'
          AND table_name = 'users'
    ) THEN
        IF NOT EXISTS (
            SELECT 1
            FROM information_schema.columns
            WHERE table_schema = 'public'
              AND table_name = 'users'
              AND column_name = 'allowed_groups'
        ) THEN
            IF NOT EXISTS (
                SELECT 1
                FROM schema_migrations
                WHERE filename = '014_drop_legacy_allowed_groups.sql'
            ) THEN
                ALTER TABLE users
                    ADD COLUMN IF NOT EXISTS allowed_groups BIGINT[] DEFAULT NULL;
            END IF;
        END IF;
    END IF;
END $$;
