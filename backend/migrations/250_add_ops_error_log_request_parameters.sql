-- Preserve explicit client parameters for diagnosing rejected effort/tier values.
-- Historical errors and requests without these parameters remain NULL.
ALTER TABLE ops_error_logs
    ADD COLUMN IF NOT EXISTS reasoning_effort TEXT,
    ADD COLUMN IF NOT EXISTS service_tier TEXT;
