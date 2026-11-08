-- 只索引带上游请求标识的行；CONCURRENTLY 避免阻塞热表写入。
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_usage_logs_upstream_request_id
    ON usage_logs (upstream_request_id)
    WHERE upstream_request_id IS NOT NULL;
