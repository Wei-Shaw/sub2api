-- MySQL 方言版本（翻译自 ../009_fix_usage_logs_cache_columns.sql）
-- Ensure usage_logs cache token columns use the underscored names expected by code.
-- Backfill from legacy column names if they exist.
--
-- 翻译说明：PG DO 块（列存在性守卫 + 条件回填）→ 临时存储过程改写。
--   - 这里的 ADD COLUMN IF NOT EXISTS 守卫必须保留：两个列在 001 已创建，
--     MySQL 不支持 IF NOT EXISTS，直接 ADD 会因列重复而失败，故先查后加；
--   - 遗留列（cache_creation5m_tokens / cache_creation1h_tokens）仅在存在时才执行
--     对应 UPDATE（存储过程体内的表/列名在执行期才解析，创建期不校验）；
--   - table_schema = 'public' → table_schema = DATABASE()。
DROP PROCEDURE IF EXISTS __mig_tmp;
CREATE PROCEDURE __mig_tmp()
BEGIN
    DECLARE v_col_count INT;

    SELECT COUNT(*) INTO v_col_count
    FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'usage_logs'
      AND column_name = 'cache_creation_5m_tokens';

    IF v_col_count = 0 THEN
        ALTER TABLE usage_logs
            ADD COLUMN cache_creation_5m_tokens INT NOT NULL DEFAULT 0;
    END IF;

    SELECT COUNT(*) INTO v_col_count
    FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'usage_logs'
      AND column_name = 'cache_creation_1h_tokens';

    IF v_col_count = 0 THEN
        ALTER TABLE usage_logs
            ADD COLUMN cache_creation_1h_tokens INT NOT NULL DEFAULT 0;
    END IF;

    -- Backfill from legacy columns if they exist.
    SELECT COUNT(*) INTO v_col_count
    FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'usage_logs'
      AND column_name = 'cache_creation5m_tokens';

    IF v_col_count > 0 THEN
        UPDATE usage_logs
        SET cache_creation_5m_tokens = cache_creation5m_tokens
        WHERE cache_creation_5m_tokens = 0
          AND cache_creation5m_tokens <> 0;
    END IF;

    SELECT COUNT(*) INTO v_col_count
    FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'usage_logs'
      AND column_name = 'cache_creation1h_tokens';

    IF v_col_count > 0 THEN
        UPDATE usage_logs
        SET cache_creation_1h_tokens = cache_creation1h_tokens
        WHERE cache_creation_1h_tokens = 0
          AND cache_creation1h_tokens <> 0;
    END IF;
END;
CALL __mig_tmp();
DROP PROCEDURE __mig_tmp;
