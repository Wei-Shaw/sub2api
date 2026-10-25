-- MySQL 方言版本（翻译自 ../153_scheduler_outbox_pending_dedup_key_index_notx.sql）
-- （PG 版为部分唯一索引 WHERE dedup_key IS NOT NULL，且 CONCURRENTLY 在线创建；
--   MySQL：唯一索引天然允许多个 NULL → 直接去掉 WHERE 谓词；
--   dedup_key 为 TEXT 列（152 添加），必须指定前缀长度。）

CREATE UNIQUE INDEX idx_scheduler_outbox_pending_dedup_key
    ON scheduler_outbox (dedup_key(191));
