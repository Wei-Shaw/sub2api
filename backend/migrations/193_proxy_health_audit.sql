-- Proxy health poller audit fields + per-group threshold overrides (Phase 2/3)
-- 代理池测活审计字段与按组阈值覆盖

-- proxies: durable health snapshot for SQL audit (Redis remains source of counters mid-flight)
ALTER TABLE proxies ADD COLUMN IF NOT EXISTS health_fail_count INT NOT NULL DEFAULT 0;
ALTER TABLE proxies ADD COLUMN IF NOT EXISTS last_health_at TIMESTAMPTZ;
ALTER TABLE proxies ADD COLUMN IF NOT EXISTS health_isolated_by VARCHAR(32);

CREATE INDEX IF NOT EXISTS proxies_last_health_at_idx ON proxies (last_health_at);
CREATE INDEX IF NOT EXISTS proxies_health_isolated_by_idx ON proxies (health_isolated_by)
    WHERE health_isolated_by IS NOT NULL AND health_isolated_by <> '';

-- proxy_groups: optional per-group thresholds (NULL / 0 = use global proxy_health config)
ALTER TABLE proxy_groups ADD COLUMN IF NOT EXISTS health_fail_threshold INT;
ALTER TABLE proxy_groups ADD COLUMN IF NOT EXISTS health_success_threshold INT;
