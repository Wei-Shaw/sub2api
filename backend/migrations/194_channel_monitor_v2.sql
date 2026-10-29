-- Passive channel monitor V2. This migration creates empty rollup tables only;
-- backfill is intentionally handled by a bounded background worker.

CREATE TABLE IF NOT EXISTS channel_monitor_v2_config (
    id SMALLINT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    version INTEGER NOT NULL DEFAULT 1,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    refresh_interval_seconds INTEGER NOT NULL DEFAULT 60
        CHECK (refresh_interval_seconds IN (60, 300)),
    platforms JSONB NOT NULL DEFAULT '[{"platform":"anthropic","enabled":true,"models":[]},{"platform":"openai","enabled":true,"models":[]},{"platform":"grok","enabled":true,"models":[]},{"platform":"kiro","enabled":true,"models":[]},{"platform":"gemini","enabled":true,"models":[]},{"platform":"antigravity","enabled":true,"models":[]}]'::jsonb,
    group_ids BIGINT[] NOT NULL DEFAULT '{}',
    updated_by BIGINT,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO channel_monitor_v2_config (id) VALUES (1)
ON CONFLICT (id) DO NOTHING;

CREATE TABLE IF NOT EXISTS channel_monitor_v2_metrics_1m (
    bucket_start TIMESTAMPTZ NOT NULL,
    platform TEXT NOT NULL,
    group_id BIGINT NOT NULL DEFAULT 0,
    model TEXT NOT NULL,
    success_requests BIGINT NOT NULL DEFAULT 0,
    error_requests BIGINT NOT NULL DEFAULT 0,
    upstream_affected_requests BIGINT NOT NULL DEFAULT 0,
    upstream_attempt_count BIGINT NOT NULL DEFAULT 0,
    input_tokens BIGINT NOT NULL DEFAULT 0,
    output_tokens BIGINT NOT NULL DEFAULT 0,
    cache_creation_tokens BIGINT NOT NULL DEFAULT 0,
    cache_read_tokens BIGINT NOT NULL DEFAULT 0,
    ttft_sum_ms BIGINT NOT NULL DEFAULT 0,
    ttft_count BIGINT NOT NULL DEFAULT 0,
    duration_sum_ms BIGINT NOT NULL DEFAULT 0,
    duration_count BIGINT NOT NULL DEFAULT 0,
    computed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (bucket_start, platform, group_id, model)
);

CREATE INDEX IF NOT EXISTS idx_channel_monitor_v2_metrics_platform_time
    ON channel_monitor_v2_metrics_1m (platform, bucket_start DESC);
CREATE INDEX IF NOT EXISTS idx_channel_monitor_v2_metrics_group_time
    ON channel_monitor_v2_metrics_1m (group_id, bucket_start DESC);
CREATE INDEX IF NOT EXISTS idx_channel_monitor_v2_metrics_model_time
    ON channel_monitor_v2_metrics_1m (model, bucket_start DESC);

CREATE TABLE IF NOT EXISTS channel_monitor_v2_user_metrics_1m (
    bucket_start TIMESTAMPTZ NOT NULL,
    platform TEXT NOT NULL,
    group_id BIGINT NOT NULL DEFAULT 0,
    model TEXT NOT NULL,
    user_id BIGINT NOT NULL,
    success_requests BIGINT NOT NULL DEFAULT 0,
    error_requests BIGINT NOT NULL DEFAULT 0,
    input_tokens BIGINT NOT NULL DEFAULT 0,
    output_tokens BIGINT NOT NULL DEFAULT 0,
    cache_creation_tokens BIGINT NOT NULL DEFAULT 0,
    cache_read_tokens BIGINT NOT NULL DEFAULT 0,
    ttft_sum_ms BIGINT NOT NULL DEFAULT 0,
    ttft_count BIGINT NOT NULL DEFAULT 0,
    duration_sum_ms BIGINT NOT NULL DEFAULT 0,
    duration_count BIGINT NOT NULL DEFAULT 0,
    computed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (bucket_start, platform, group_id, model, user_id)
);

CREATE INDEX IF NOT EXISTS idx_channel_monitor_v2_user_metrics_user_time
    ON channel_monitor_v2_user_metrics_1m (user_id, bucket_start DESC);
CREATE INDEX IF NOT EXISTS idx_channel_monitor_v2_user_metrics_time
    ON channel_monitor_v2_user_metrics_1m (bucket_start DESC);

CREATE TABLE IF NOT EXISTS channel_monitor_v2_error_metrics_1m (
    bucket_start TIMESTAMPTZ NOT NULL,
    platform TEXT NOT NULL,
    group_id BIGINT NOT NULL DEFAULT 0,
    model TEXT NOT NULL,
    error_category TEXT NOT NULL,
    taxonomy_version SMALLINT NOT NULL,
    error_requests BIGINT NOT NULL DEFAULT 0,
    PRIMARY KEY (bucket_start, platform, group_id, model, error_category, taxonomy_version)
);

CREATE INDEX IF NOT EXISTS idx_channel_monitor_v2_errors_time
    ON channel_monitor_v2_error_metrics_1m (bucket_start DESC);
CREATE INDEX IF NOT EXISTS idx_channel_monitor_v2_errors_category_time
    ON channel_monitor_v2_error_metrics_1m (error_category, bucket_start DESC);

CREATE TABLE IF NOT EXISTS channel_monitor_v2_latency_histograms_1m (
    bucket_start TIMESTAMPTZ NOT NULL,
    platform TEXT NOT NULL,
    group_id BIGINT NOT NULL DEFAULT 0,
    model TEXT NOT NULL,
    user_id BIGINT NOT NULL DEFAULT 0,
    metric TEXT NOT NULL CHECK (metric IN ('ttft', 'duration')),
    upper_bound_ms INTEGER NOT NULL,
    sample_count BIGINT NOT NULL DEFAULT 0,
    PRIMARY KEY (bucket_start, platform, group_id, model, user_id, metric, upper_bound_ms)
);

CREATE INDEX IF NOT EXISTS idx_channel_monitor_v2_histograms_time
    ON channel_monitor_v2_latency_histograms_1m (bucket_start DESC, metric);

CREATE TABLE IF NOT EXISTS channel_monitor_v2_watermarks (
    id SMALLINT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    usage_coverage_start TIMESTAMPTZ,
    error_coverage_start TIMESTAMPTZ,
    data_through TIMESTAMPTZ,
    last_successful_at TIMESTAMPTZ,
    backfill_cursor TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO channel_monitor_v2_watermarks (id) VALUES (1)
ON CONFLICT (id) DO NOTHING;

COMMENT ON TABLE channel_monitor_v2_metrics_1m IS
    'One-minute passive channel health facts derived from real user requests; never from active probes.';
COMMENT ON COLUMN channel_monitor_v2_error_metrics_1m.taxonomy_version IS
    'Version of the ordered error classification rules used for this aggregate.';
