-- usage_billing_dedup 是按时间追加写入的幂等窄表。
-- 使用 BRIN 支撑按 created_at 的批量保留期清理，尽量降低写放大。
-- 使用 CONCURRENTLY 避免在热表上长时间阻塞写入。

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_usage_billing_dedup_created_at_brin
    ON usage_billing_dedup
    USING BRIN (created_at);
