-- MySQL 方言版本（翻译自 ../006b_guard_users_allowed_groups.sql）
-- 兼容缺失 users.allowed_groups 的老库，确保 007 回填可执行。
--
-- 翻译说明：PG DO 块（users 表存在性 + allowed_groups 列存在性 + schema_migrations
--           是否已记录 014 三重守卫）→ 临时存储过程改写；
--           table_schema = 'public' → table_schema = DATABASE()；
--           BIGINT[] DEFAULT NULL → JSON DEFAULT NULL；
--           ADD COLUMN IF NOT EXISTS → 先查 information_schema.columns 再 ADD。
DROP PROCEDURE IF EXISTS __mig_tmp;
CREATE PROCEDURE __mig_tmp()
BEGIN
    DECLARE v_users_count INT;
    DECLARE v_col_count INT;
    DECLARE v_mig_count INT;

    SELECT COUNT(*) INTO v_users_count
    FROM information_schema.tables
    WHERE table_schema = DATABASE()
      AND table_name = 'users';

    IF v_users_count > 0 THEN
        SELECT COUNT(*) INTO v_col_count
        FROM information_schema.columns
        WHERE table_schema = DATABASE()
          AND table_name = 'users'
          AND column_name = 'allowed_groups';

        IF v_col_count = 0 THEN
            -- 014 已执行过则说明 legacy 列是被有意删除的，不再补回
            SELECT COUNT(*) INTO v_mig_count
            FROM schema_migrations
            WHERE filename = '014_drop_legacy_allowed_groups.sql';

            IF v_mig_count = 0 THEN
                ALTER TABLE users
                    ADD COLUMN allowed_groups JSON DEFAULT NULL;
            END IF;
        END IF;
    END IF;
END;
CALL __mig_tmp();
DROP PROCEDURE __mig_tmp;
