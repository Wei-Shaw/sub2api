-- MySQL 方言版本（翻译自 ../152_scheduler_outbox_dedup_key.sql）
-- 翻译说明：ADD COLUMN IF NOT EXISTS → ADD COLUMN（按迁移执行器单次执行语义去掉）。
-- 后续 153 的唯一索引对本 TEXT 列使用 191 前缀长度。

ALTER TABLE scheduler_outbox
    ADD COLUMN dedup_key TEXT;
