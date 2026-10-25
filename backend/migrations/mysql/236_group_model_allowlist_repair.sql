-- MySQL 方言版本（翻译自 ../236_group_model_allowlist_repair.sql）
-- 翻译说明：
--   - 本迁移的语义是「可重放的修复」。235 在 MySQL 侧按无条件改名翻译后，正常链路中
--     model_allowlist 必然存在、models_list_config 不存在，DO 块各分支均为 no-op；
--     但为保留「回滚旧镜像 / 部分恢复旧结构备份」场景下的修复能力（语义对等优先），
--     存在性分支不省略，改用 information_schema.columns 检查（pg_attribute →
--     information_schema.columns，table_schema = 'public' → DATABASE()），
--     在临时存储过程内执行（CALL 后立即 DROP，不写 DELIMITER）。
--   - ADD COLUMN IF NOT EXISTS / UPDATE ... IS NULL / SET DEFAULT / SET NOT NULL
--     一并移入过程内，避免列已存在时 ADD COLUMN 报 1060 重复列错误。
--   - jsonb '{}' 字面量 → CAST('{}' AS JSON)（COALESCE/比较均按 JSON 值进行）。
--   - `groups` 为 MySQL 8 保留字，加反引号；COMMENT ON COLUMN 整句跳过。

-- 236: 收敛 groups 的模型白名单列，修复 235 未落地却已被记账的实例。
--   1) 只有旧列              -> 重命名，数据原样保留；
--   2) 两列并存              -> 新列仍是默认空值时，把旧列里的配置回填过来，旧列保留不动；
--   3) 两列都没有            -> 按默认值补建新列。
-- 结束后保证 groups.model_allowlist 一定存在，且为 NOT NULL DEFAULT '{}'。
DROP PROCEDURE IF EXISTS __migration_236_tmp;

CREATE PROCEDURE __migration_236_tmp()
main: BEGIN
    DECLARE v_old_exists INT DEFAULT 0;
    DECLARE v_new_exists INT DEFAULT 0;

    SELECT COUNT(*) INTO v_old_exists
      FROM information_schema.columns
     WHERE table_schema = DATABASE()
       AND table_name = 'groups'
       AND column_name = 'models_list_config';

    SELECT COUNT(*) INTO v_new_exists
      FROM information_schema.columns
     WHERE table_schema = DATABASE()
       AND table_name = 'groups'
       AND column_name = 'model_allowlist';

    -- 1) 只有旧列 -> 重命名，数据原样保留
    IF v_old_exists = 1 AND v_new_exists = 0 THEN
        ALTER TABLE `groups` RENAME COLUMN models_list_config TO model_allowlist;
        SET v_new_exists = 1;
    -- 2) 两列并存 -> 新列仍是默认空值时，把旧列里的配置回填过来，旧列保留不动
    ELSEIF v_old_exists = 1 AND v_new_exists = 1 THEN
        UPDATE `groups`
           SET model_allowlist = models_list_config
         WHERE COALESCE(model_allowlist, CAST('{}' AS JSON)) = CAST('{}' AS JSON)
           AND COALESCE(models_list_config, CAST('{}' AS JSON)) <> CAST('{}' AS JSON);
    END IF;

    -- 3) 两列都没有 -> 按默认值补建新列
    IF v_new_exists = 0 THEN
        ALTER TABLE `groups`
            ADD COLUMN model_allowlist JSON NOT NULL DEFAULT ('{}');
    END IF;

    UPDATE `groups`
       SET model_allowlist = CAST('{}' AS JSON)
     WHERE model_allowlist IS NULL;

    -- MySQL 的 ALTER COLUMN SET DEFAULT 不支持 JSON 表达式默认值（错误 1101），
    -- 统一用 MODIFY COLUMN 完整定义（NOT NULL + 表达式默认值）。
    ALTER TABLE `groups` MODIFY COLUMN model_allowlist JSON NOT NULL DEFAULT ('{}');
END;

CALL __migration_236_tmp();
DROP PROCEDURE __migration_236_tmp;

-- (comment skipped) COMMENT ON COLUMN groups.model_allowlist IS
--     'Group model allowlist: constrains both model listing responses and request admission'
