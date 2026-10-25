-- MySQL 方言版本（翻译自 ../011_remove_duplicate_unique_indexes.sql）
-- 011_remove_duplicate_unique_indexes.sql
-- 移除重复的唯一索引
-- 这些字段在 ent schema 的 Fields() 中已声明 .Unique()，
-- 因此在 Indexes() 中再次声明 index.Fields("x").Unique() 会创建重复索引。
-- 本迁移脚本清理这些冗余索引。
--
-- 重复索引命名约定（由 Ent 自动生成/历史迁移遗留）:
-- - 字段级 Unique() 创建的索引名: <table>_<field>_key
-- - Indexes() 中的 Unique() 创建的索引名: <table>_<field>
-- - 初始化迁移中的非唯一索引: idx_<table>_<field>
--
-- 翻译说明：PG 的 DROP INDEX IF EXISTS（候选名至多存在其一，需幂等跳过不存在的）
--   → 临时存储过程 + information_schema.statistics 存在性检查后逐个 DROP INDEX。
--   （MySQL 8 无 DROP INDEX IF EXISTS 语法；存储过程体内静态 DDL 在执行期才解析，
--   仅在索引确实存在时执行对应分支。）

DROP PROCEDURE IF EXISTS __mig_tmp;
CREATE PROCEDURE __mig_tmp()
BEGIN
    DECLARE v_idx_count INT;

    -- api_keys 表: key 字段
    SELECT COUNT(*) INTO v_idx_count FROM information_schema.statistics
    WHERE table_schema = DATABASE() AND table_name = 'api_keys' AND index_name = 'apikey_key';
    IF v_idx_count > 0 THEN
        DROP INDEX apikey_key ON api_keys;
    END IF;

    SELECT COUNT(*) INTO v_idx_count FROM information_schema.statistics
    WHERE table_schema = DATABASE() AND table_name = 'api_keys' AND index_name = 'api_keys_key';
    IF v_idx_count > 0 THEN
        DROP INDEX api_keys_key ON api_keys;
    END IF;

    SELECT COUNT(*) INTO v_idx_count FROM information_schema.statistics
    WHERE table_schema = DATABASE() AND table_name = 'api_keys' AND index_name = 'idx_api_keys_key';
    IF v_idx_count > 0 THEN
        DROP INDEX idx_api_keys_key ON api_keys;
    END IF;

    -- users 表: email 字段
    SELECT COUNT(*) INTO v_idx_count FROM information_schema.statistics
    WHERE table_schema = DATABASE() AND table_name = 'users' AND index_name = 'user_email';
    IF v_idx_count > 0 THEN
        DROP INDEX user_email ON users;
    END IF;

    SELECT COUNT(*) INTO v_idx_count FROM information_schema.statistics
    WHERE table_schema = DATABASE() AND table_name = 'users' AND index_name = 'users_email';
    IF v_idx_count > 0 THEN
        DROP INDEX users_email ON users;
    END IF;

    SELECT COUNT(*) INTO v_idx_count FROM information_schema.statistics
    WHERE table_schema = DATABASE() AND table_name = 'users' AND index_name = 'idx_users_email';
    IF v_idx_count > 0 THEN
        DROP INDEX idx_users_email ON users;
    END IF;

    -- settings 表: key 字段
    SELECT COUNT(*) INTO v_idx_count FROM information_schema.statistics
    WHERE table_schema = DATABASE() AND table_name = 'settings' AND index_name = 'settings_key';
    IF v_idx_count > 0 THEN
        DROP INDEX settings_key ON settings;
    END IF;

    SELECT COUNT(*) INTO v_idx_count FROM information_schema.statistics
    WHERE table_schema = DATABASE() AND table_name = 'settings' AND index_name = 'idx_settings_key';
    IF v_idx_count > 0 THEN
        DROP INDEX idx_settings_key ON settings;
    END IF;

    -- redeem_codes 表: code 字段
    SELECT COUNT(*) INTO v_idx_count FROM information_schema.statistics
    WHERE table_schema = DATABASE() AND table_name = 'redeem_codes' AND index_name = 'redeemcode_code';
    IF v_idx_count > 0 THEN
        DROP INDEX redeemcode_code ON redeem_codes;
    END IF;

    SELECT COUNT(*) INTO v_idx_count FROM information_schema.statistics
    WHERE table_schema = DATABASE() AND table_name = 'redeem_codes' AND index_name = 'redeem_codes_code';
    IF v_idx_count > 0 THEN
        DROP INDEX redeem_codes_code ON redeem_codes;
    END IF;

    SELECT COUNT(*) INTO v_idx_count FROM information_schema.statistics
    WHERE table_schema = DATABASE() AND table_name = 'redeem_codes' AND index_name = 'idx_redeem_codes_code';
    IF v_idx_count > 0 THEN
        DROP INDEX idx_redeem_codes_code ON redeem_codes;
    END IF;

    -- groups 表: name 字段（`groups` 为 MySQL 8 保留字）
    SELECT COUNT(*) INTO v_idx_count FROM information_schema.statistics
    WHERE table_schema = DATABASE() AND table_name = 'groups' AND index_name = 'group_name';
    IF v_idx_count > 0 THEN
        DROP INDEX group_name ON `groups`;
    END IF;

    SELECT COUNT(*) INTO v_idx_count FROM information_schema.statistics
    WHERE table_schema = DATABASE() AND table_name = 'groups' AND index_name = 'groups_name';
    IF v_idx_count > 0 THEN
        DROP INDEX groups_name ON `groups`;
    END IF;

    SELECT COUNT(*) INTO v_idx_count FROM information_schema.statistics
    WHERE table_schema = DATABASE() AND table_name = 'groups' AND index_name = 'idx_groups_name';
    IF v_idx_count > 0 THEN
        DROP INDEX idx_groups_name ON `groups`;
    END IF;
END;
CALL __mig_tmp();
DROP PROCEDURE __mig_tmp;

-- 注意: 每个字段的唯一约束仍由字段级 Unique() 创建的约束保留，
-- 如 api_keys_key_key、users_email_key 等。
-- （MySQL 侧对应 001 内联 UNIQUE 生成的唯一索引：api_keys.`key`、users.email、
--   settings.`key`、redeem_codes.code、`groups`.name；016 迁移再改部分唯一索引。）
