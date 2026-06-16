-- Migration: Move wechat field from users table to user_attribute_values
-- This migration:
-- 1. Creates a "wechat" attribute definition
-- 2. Migrates existing wechat data to user_attribute_values
-- 3. Does NOT drop the wechat column (for rollback safety, can be done in a later migration)

-- +goose Up
-- +goose StatementBegin

-- Step 1: Insert wechat attribute definition if not exists
INSERT INTO user_attribute_definitions (key, name, description, type, options, required, validation, placeholder, display_order, enabled, created_at, updated_at)
SELECT 'wechat', '微信', '用户微信号', 'text', '[]'::jsonb, false, '{}'::jsonb, '请输入微信号', 0, true, NOW(), NOW()
WHERE NOT EXISTS (
    SELECT 1 FROM user_attribute_definitions WHERE key = 'wechat' AND deleted_at IS NULL
);

-- Step 2: Migrate existing wechat values to user_attribute_values
-- Only migrate non-empty values
INSERT INTO user_attribute_values (user_id, attribute_id, value, created_at, updated_at)
SELECT
    u.id,
    (SELECT id FROM user_attribute_definitions WHERE key = 'wechat' AND deleted_at IS NULL LIMIT 1),
    u.wechat,
    NOW(),
    NOW()
FROM users u
WHERE u.wechat IS NOT NULL
  AND u.wechat != ''
  AND u.deleted_at IS NULL
  AND NOT EXISTS (
      SELECT 1 FROM user_attribute_values uav
      WHERE uav.user_id = u.id
        AND uav.attribute_id = (SELECT id FROM user_attribute_definitions WHERE key = 'wechat' AND deleted_at IS NULL LIMIT 1)
  );

-- Step 3: Update display_order to ensure wechat appears first
UPDATE user_attribute_definitions
SET display_order = -1
WHERE key = 'wechat' AND deleted_at IS NULL;

-- Reorder all attributes starting from 0
WITH ordered AS (
    SELECT id, ROW_NUMBER() OVER (ORDER BY display_order, id) - 1 as new_order
    FROM user_attribute_definitions
    WHERE deleted_at IS NULL
)
UPDATE user_attribute_definitions
SET display_order = ordered.new_order
FROM ordered
WHERE user_attribute_definitions.id = ordered.id;

-- Step 4: Drop the redundant wechat column from users table
ALTER TABLE users DROP COLUMN IF EXISTS wechat;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- Restore wechat column
ALTER TABLE users ADD COLUMN IF NOT EXISTS wechat VARCHAR(100) DEFAULT '';

-- Copy attribute values back to users.wechat column
UPDATE users u
SET wechat = uav.value
FROM user_attribute_values uav
JOIN user_attribute_definitions uad ON uav.attribute_id = uad.id
WHERE uav.user_id = u.id
  AND uad.key = 'wechat'
  AND uad.deleted_at IS NULL;

-- Delete migrated attribute values
DELETE FROM user_attribute_values
WHERE attribute_id IN (
    SELECT id FROM user_attribute_definitions WHERE key = 'wechat' AND deleted_at IS NULL
);

-- Soft-delete the wechat attribute definition
UPDATE user_attribute_definitions
SET deleted_at = NOW()
WHERE key = 'wechat' AND deleted_at IS NULL;

-- +goose StatementEnd
