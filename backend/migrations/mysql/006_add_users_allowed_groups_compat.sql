-- MySQL 方言版本（翻译自 ../006_add_users_allowed_groups_compat.sql）
-- 兼容旧库：若尚未创建 user_allowed_groups，则确保 users.allowed_groups 存在，避免 007 迁移回填失败。
--
-- 翻译说明：PG DO 块（to_regclass / information_schema 存在性守卫）→ 临时存储过程改写；
--           table_schema = 'public' → table_schema = DATABASE()；
--           BIGINT[] DEFAULT NULL → JSON DEFAULT NULL；
--           ADD COLUMN IF NOT EXISTS → 先查 information_schema.columns 再 ADD（MySQL 不支持该修饰词）。
DROP PROCEDURE IF EXISTS __mig_tmp;
CREATE PROCEDURE __mig_tmp()
BEGIN
    DECLARE v_uag_count INT;
    DECLARE v_users_count INT;
    DECLARE v_col_count INT;

    -- user_allowed_groups 表不存在（007 尚未执行）时才需要补列
    SELECT COUNT(*) INTO v_uag_count
    FROM information_schema.tables
    WHERE table_schema = DATABASE()
      AND table_name = 'user_allowed_groups';

    IF v_uag_count = 0 THEN
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
                ALTER TABLE users
                    ADD COLUMN allowed_groups JSON DEFAULT NULL;
            END IF;
        END IF;
    END IF;
END;
CALL __mig_tmp();
DROP PROCEDURE __mig_tmp;
