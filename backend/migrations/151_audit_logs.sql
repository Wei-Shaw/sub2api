CREATE TABLE IF NOT EXISTS audit_logs (
    id BIGSERIAL PRIMARY KEY,
    request_id VARCHAR(64) NOT NULL,
    user_id BIGINT,
    user_email VARCHAR(255) DEFAULT '',
    api_key_id BIGINT,
    api_key_name VARCHAR(255) DEFAULT '',
    group_id BIGINT,
    group_name VARCHAR(255) DEFAULT '',
    platform VARCHAR(50) DEFAULT '',
    endpoint VARCHAR(255) NOT NULL DEFAULT '',
    method VARCHAR(16) NOT NULL DEFAULT '',
    model VARCHAR(255) DEFAULT '',
    status_code INTEGER NOT NULL DEFAULT 0,
    request_body TEXT NOT NULL DEFAULT '',
    response_body TEXT NOT NULL DEFAULT '',
    request_truncated BOOLEAN NOT NULL DEFAULT FALSE,
    response_truncated BOOLEAN NOT NULL DEFAULT FALSE,
    duration_ms INTEGER NOT NULL DEFAULT 0,
    ip_address VARCHAR(45) DEFAULT '',
    user_agent VARCHAR(512) DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_audit_logs_created_at ON audit_logs (created_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_logs_request_id ON audit_logs (request_id);
CREATE INDEX IF NOT EXISTS idx_audit_logs_user_id_created_at ON audit_logs (user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_logs_api_key_id_created_at ON audit_logs (api_key_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_logs_platform_created_at ON audit_logs (platform, created_at DESC);
