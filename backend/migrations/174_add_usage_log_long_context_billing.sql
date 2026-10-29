-- Snapshot whether long-context pricing changed token prices for a request so
-- usage history can explain the applied charge without inferring from totals.
ALTER TABLE usage_logs
    ADD COLUMN IF NOT EXISTS long_context_billing_applied BOOLEAN NOT NULL DEFAULT FALSE;
