-- 迁移：为 api_keys 增加 last_used_at 字段，用于记录 API Key 最近使用时间
-- 幂等执行：可重复运行

ALTER TABLE api_keys
ADD COLUMN IF NOT EXISTS last_used_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_api_keys_last_used_at
ON api_keys(last_used_at)
WHERE deleted_at IS NULL;
