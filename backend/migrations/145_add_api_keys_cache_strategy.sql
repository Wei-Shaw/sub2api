-- Add cache_strategy to api_keys for user-level cache TTL preference.
-- Values: "auto" (inherit account/global), "cost_priority" (force 5m),
--         "latency_priority" (force 1h). See Account.SupportsCachePolicy()
-- for the capability gate. Existing rows backfilled to "auto".
ALTER TABLE api_keys
    ADD COLUMN IF NOT EXISTS cache_strategy VARCHAR(32) NOT NULL DEFAULT 'auto';
