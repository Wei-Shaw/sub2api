-- Proxy-pool health is only valid after the proxy reaches the Grok API
-- quality target. Existing generic-connectivity snapshots must be rechecked.

ALTER TABLE proxies
    ADD COLUMN IF NOT EXISTS pool_grok_quality_status VARCHAR(20) NOT NULL DEFAULT 'unknown';
ALTER TABLE proxies
    ADD COLUMN IF NOT EXISTS pool_grok_quality_checked_at TIMESTAMPTZ;
ALTER TABLE proxies
    ADD COLUMN IF NOT EXISTS pool_grok_quality_http_status INT;
ALTER TABLE proxies
    ADD COLUMN IF NOT EXISTS pool_grok_quality_message TEXT;

UPDATE proxies
SET pool_health = 'unknown',
    pool_checked_at = NULL,
    pool_failures = 0,
    pool_grok_quality_status = 'unknown',
    pool_grok_quality_checked_at = NULL,
    pool_grok_quality_http_status = NULL,
    pool_grok_quality_message = NULL,
    updated_at = NOW()
WHERE pool_id IS NOT NULL;
