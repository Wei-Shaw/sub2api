-- MySQL 方言版本（翻译自 ../056_add_api_key_last_used_at.sql）
-- 迁移：为 api_keys 增加 last_used_at 字段，用于记录 API Key 最近使用时间
-- 幂等执行：可重复运行（由迁移执行器保证单次执行）

ALTER TABLE api_keys
ADD COLUMN last_used_at DATETIME(3) NULL;

-- PG 版为部分索引 WHERE deleted_at IS NULL；非唯一部分索引 → 去掉 WHERE
CREATE INDEX idx_api_keys_last_used_at
ON api_keys(last_used_at);
