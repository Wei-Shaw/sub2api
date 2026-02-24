-- 幂等记录表：用于关键写接口的请求去重与结果重放
-- 幂等执行：可重复运行

CREATE TABLE IF NOT EXISTS idempotency_records (
    id BIGSERIAL PRIMARY KEY,
    scope VARCHAR(128) NOT NULL,
    idempotency_key_hash VARCHAR(64) NOT NULL,
    request_fingerprint VARCHAR(64) NOT NULL,
    status VARCHAR(32) NOT NULL,
    response_status INTEGER,
    response_body TEXT,
    error_reason VARCHAR(128),
    locked_until TIMESTAMPTZ,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_idempotency_records_scope_key
    ON idempotency_records (scope, idempotency_key_hash);

CREATE INDEX IF NOT EXISTS idx_idempotency_records_expires_at
    ON idempotency_records (expires_at);

CREATE INDEX IF NOT EXISTS idx_idempotency_records_status_locked_until
    ON idempotency_records (status, locked_until);

