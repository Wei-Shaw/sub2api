-- MySQL 方言版本（翻译自 ../238b_content_moderation_engine_meta.sql）
-- 翻译说明：JSONB → JSON（可空）；ADD COLUMN IF NOT EXISTS
-- 按迁移执行器单次执行语义去掉修饰词。

-- Audit provenance only; old rows and old application writes remain nullable.
ALTER TABLE content_moderation_logs ADD COLUMN engine_meta JSON NULL;
