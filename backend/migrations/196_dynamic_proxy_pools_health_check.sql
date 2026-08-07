-- Add health_check_interval_sec column for dynamic pool proxy health checking
ALTER TABLE dynamic_proxy_pools
    ADD COLUMN IF NOT EXISTS health_check_interval_sec INT NOT NULL DEFAULT 0;