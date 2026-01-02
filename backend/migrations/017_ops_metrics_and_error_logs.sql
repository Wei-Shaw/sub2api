-- Ops error logs and system metrics

CREATE TABLE IF NOT EXISTS ops_error_logs (
    id BIGSERIAL PRIMARY KEY,
    request_id VARCHAR(64),
    user_id BIGINT,
    api_key_id BIGINT,
    account_id BIGINT,
    group_id BIGINT,
    client_ip INET,
    error_phase VARCHAR(32) NOT NULL,
    error_type VARCHAR(64) NOT NULL,
    severity VARCHAR(4) NOT NULL,
    status_code INT,
    platform VARCHAR(32),
    model VARCHAR(100),
    request_path VARCHAR(256),
    stream BOOLEAN NOT NULL DEFAULT FALSE,
    error_message TEXT,
    error_body TEXT,
    provider_error_code VARCHAR(64),
    provider_error_type VARCHAR(64),
    is_retryable BOOLEAN NOT NULL DEFAULT FALSE,
    is_user_actionable BOOLEAN NOT NULL DEFAULT FALSE,
    retry_count INT NOT NULL DEFAULT 0,
    completion_status VARCHAR(16),
    duration_ms INT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ops_error_logs_created_at ON ops_error_logs (created_at DESC);
CREATE INDEX IF NOT EXISTS idx_ops_error_logs_phase ON ops_error_logs (error_phase);
CREATE INDEX IF NOT EXISTS idx_ops_error_logs_platform ON ops_error_logs (platform);
CREATE INDEX IF NOT EXISTS idx_ops_error_logs_severity ON ops_error_logs (severity);
CREATE INDEX IF NOT EXISTS idx_ops_error_logs_phase_platform_time ON ops_error_logs (error_phase, platform, created_at DESC);

CREATE TABLE IF NOT EXISTS ops_system_metrics (
    id BIGSERIAL PRIMARY KEY,
    success_rate DOUBLE PRECISION,
    error_rate DOUBLE PRECISION,
    p95_latency_ms INT,
    p99_latency_ms INT,
    http2_errors INT,
    active_alerts INT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ops_system_metrics_created_at ON ops_system_metrics (created_at DESC);
