-- MySQL 方言版本（翻译自 ../183_ops_ingress_reject_aggregates.sql）
--
-- 说明：
--   - PG 的 SET LOCAL lock_timeout / statement_timeout 为会话级守护参数，MySQL 无对应机制，整句跳过
--     （迁移执行器自身保证超时控制）。
--   - client_ip 的 INET 类型 → VARCHAR(45)（可容纳 IPv6 全格式，与 033 迁移口径一致）。

CREATE TABLE IF NOT EXISTS ops_ingress_reject_aggregates (
    id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    bucket_start DATETIME(3) NOT NULL,
    reject_reason VARCHAR(64) NOT NULL,
    route_family VARCHAR(64) NOT NULL,
    protocol VARCHAR(32) NOT NULL,
    client_ip VARCHAR(45) NOT NULL,
    user_id BIGINT NOT NULL DEFAULT 0,
    api_key_id BIGINT NOT NULL DEFAULT 0,
    request_count BIGINT NOT NULL DEFAULT 0,
    first_seen DATETIME(3) NOT NULL,
    last_seen DATETIME(3) NOT NULL,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    CONSTRAINT ops_ingress_reject_aggregates_dimensions_unique UNIQUE
        (bucket_start, reject_reason, route_family, protocol, client_ip, user_id, api_key_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE INDEX idx_ops_ingress_reject_aggregates_bucket
    ON ops_ingress_reject_aggregates (bucket_start DESC);
CREATE INDEX idx_ops_ingress_reject_aggregates_reason_bucket
    ON ops_ingress_reject_aggregates (reject_reason, bucket_start DESC);
CREATE INDEX idx_ops_ingress_reject_aggregates_ip_bucket
    ON ops_ingress_reject_aggregates (client_ip, bucket_start DESC);
