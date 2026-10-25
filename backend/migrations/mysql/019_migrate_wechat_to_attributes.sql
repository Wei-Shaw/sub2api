-- MySQL 方言版本（翻译自 ../019_migrate_wechat_to_attributes.sql）
-- Migration: Move wechat field from users table to user_attribute_values
-- This migration:
-- 1. Creates a "wechat" attribute definition
-- 2. Migrates existing wechat data to user_attribute_values
-- 3. Does NOT drop the wechat column (for rollback safety, can be done in a later migration)
--
-- 翻译说明：
--   - goose 指令注释（+goose Up/Down/StatementBegin/StatementEnd）非 SQL 语义，翻译时去掉；
--     执行器整文件执行，Down 段不能保留，故不翻译；
--   - `key` 为 MySQL 保留字，全程反引号；
--   - jsonb 字面量 '[]'::jsonb / '{}'::jsonb → CAST(... AS JSON)；
--   - PG 无 FROM 的 INSERT...SELECT → FROM DUAL；
--   - WITH ordered AS (...) UPDATE ... FROM ordered → UPDATE ... JOIN (派生表)（多表 UPDATE
--     对同表派生表必定物化，规避 MySQL 1093 错误，语义与 PG CTE 版一致）；
--   - UPDATE ... FROM → UPDATE ... JOIN；NOW() → NOW(3)。

-- Step 1: Insert wechat attribute definition if not exists
INSERT INTO user_attribute_definitions (`key`, name, description, type, options, required, validation, placeholder, display_order, enabled, created_at, updated_at)
SELECT 'wechat', '微信', '用户微信号', 'text', CAST('[]' AS JSON), FALSE, CAST('{}' AS JSON), '请输入微信号', 0, TRUE, NOW(3), NOW(3)
FROM DUAL
WHERE NOT EXISTS (
    SELECT 1 FROM user_attribute_definitions WHERE `key` = 'wechat' AND deleted_at IS NULL
);

-- Step 2: Migrate existing wechat values to user_attribute_values
-- Only migrate non-empty values
INSERT INTO user_attribute_values (user_id, attribute_id, value, created_at, updated_at)
SELECT
    u.id,
    (SELECT id FROM user_attribute_definitions WHERE `key` = 'wechat' AND deleted_at IS NULL LIMIT 1),
    u.wechat,
    NOW(3),
    NOW(3)
FROM users u
WHERE u.wechat IS NOT NULL
  AND u.wechat <> ''
  AND u.deleted_at IS NULL
  AND NOT EXISTS (
      SELECT 1 FROM user_attribute_values uav
      WHERE uav.user_id = u.id
        AND uav.attribute_id = (SELECT id FROM user_attribute_definitions WHERE `key` = 'wechat' AND deleted_at IS NULL LIMIT 1)
  );

-- Step 3: Update display_order to ensure wechat appears first
UPDATE user_attribute_definitions
SET display_order = -1
WHERE `key` = 'wechat' AND deleted_at IS NULL;

-- Reorder all attributes starting from 0
UPDATE user_attribute_definitions uad
JOIN (
    SELECT id, ROW_NUMBER() OVER (ORDER BY display_order, id) - 1 AS new_order
    FROM user_attribute_definitions
    WHERE deleted_at IS NULL
) ordered ON uad.id = ordered.id
SET uad.display_order = ordered.new_order;

-- Step 4: Drop the redundant wechat column from users table
ALTER TABLE users DROP COLUMN wechat;

-- （PG 版 Down 段不翻译：恢复 wechat 列、回拷数据、删除迁移的属性值、软删定义。）
