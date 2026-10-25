-- MySQL 方言版本（翻译自 ../072_add_usage_billing_dedup_created_at_brin_notx.sql）
-- usage_billing_dedup 是按时间追加写入的幂等窄表。
-- （PG 使用 BRIN 支撑按 created_at 的批量保留期清理，并 CONCURRENTLY 避免阻塞；
--   MySQL 无 BRIN/CONCURRENTLY，改为普通 B-tree 索引。）

CREATE INDEX idx_usage_billing_dedup_created_at_brin
    ON usage_billing_dedup (created_at);
