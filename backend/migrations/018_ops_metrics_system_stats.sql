-- Extend ops_system_metrics with windowed/system stats

ALTER TABLE ops_system_metrics
    ADD COLUMN IF NOT EXISTS window_minutes INT NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS cpu_usage_percent DOUBLE PRECISION,
    ADD COLUMN IF NOT EXISTS memory_used_mb BIGINT,
    ADD COLUMN IF NOT EXISTS memory_total_mb BIGINT,
    ADD COLUMN IF NOT EXISTS memory_usage_percent DOUBLE PRECISION,
    ADD COLUMN IF NOT EXISTS heap_alloc_mb BIGINT,
    ADD COLUMN IF NOT EXISTS gc_pause_ms DOUBLE PRECISION,
    ADD COLUMN IF NOT EXISTS concurrency_queue_depth INT;

CREATE INDEX IF NOT EXISTS idx_ops_system_metrics_window_time
    ON ops_system_metrics (window_minutes, created_at DESC);
