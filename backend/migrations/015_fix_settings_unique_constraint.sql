-- 015_fix_settings_unique_constraint.sql
-- 修复 settings 表 key 字段缺失的唯一约束
-- 此约束是 ON CONFLICT ("key") DO UPDATE 语句所必需的

-- 检查并添加唯一约束（如果不存在）
DO $$
BEGIN
    -- 检查是否已存在唯一约束
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conrelid = 'settings'::regclass
        AND contype = 'u'
        AND conname = 'settings_key_key'
    ) THEN
        -- 添加唯一约束
        ALTER TABLE settings ADD CONSTRAINT settings_key_key UNIQUE (key);
    END IF;
END
$$;
