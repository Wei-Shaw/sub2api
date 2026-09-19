-- Keep prior successful migration checksums intact when extending billing.
ALTER TABLE upstream_billing_connections
    ADD COLUMN IF NOT EXISTS sync_lookback_months INTEGER NOT NULL DEFAULT 2
        CHECK (sync_lookback_months BETWEEN 1 AND 6),
    ADD COLUMN IF NOT EXISTS allocation_mode VARCHAR(32) NOT NULL DEFAULT 'local_weighted'
        CHECK (allocation_mode IN ('local_weighted', 'official_only', 'disabled'));

ALTER TABLE upstream_billing_runs
    ADD COLUMN IF NOT EXISTS allocation_mode VARCHAR(32) NOT NULL DEFAULT 'local_weighted'
        CHECK (allocation_mode IN ('local_weighted', 'official_only', 'disabled'));
