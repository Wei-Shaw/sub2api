-- 窄表账务幂等键：将“是否已扣费”从 usage_logs 解耦出来
-- 幂等执行：可重复运行

CREATE TABLE IF NOT EXISTS usage_billing_dedup (
    id BIGSERIAL PRIMARY KEY,
    request_id VARCHAR(255) NOT NULL,
    api_key_id BIGINT NOT NULL,
    request_fingerprint VARCHAR(64) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_usage_billing_dedup_request_api_key
    ON usage_billing_dedup (request_id, api_key_id);
