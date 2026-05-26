-- 兼容旧库：若尚未创建 user_allowed_groups，则确保 users.allowed_groups 存在，避免 007 迁移回填失败。
DO $$
BEGIN
    IF to_regclass('public.user_allowed_groups') IS NULL THEN
        IF EXISTS (
            SELECT 1
            FROM information_schema.tables
            WHERE table_schema = 'public'
              AND table_name = 'users'
        ) THEN
            ALTER TABLE users
                ADD COLUMN IF NOT EXISTS allowed_groups BIGINT[] DEFAULT NULL;
        END IF;
    END IF;
END $$;
