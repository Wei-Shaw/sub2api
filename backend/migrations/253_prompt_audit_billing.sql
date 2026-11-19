-- Prompt-audit fees billed to users.
-- usage_logs.prompt_audit_cost: the audit fee charged with a forwarded request,
-- at list price and already included in actual_cost.
-- prompt_audit_events.audit_*: what the guard calls of one evaluation used and
-- cost; for a blocked request this is the separately charged fee.
ALTER TABLE usage_logs
    ADD COLUMN IF NOT EXISTS prompt_audit_cost NUMERIC(20,10) NOT NULL DEFAULT 0;

ALTER TABLE prompt_audit_events
    ADD COLUMN IF NOT EXISTS audit_input_tokens INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS audit_cached_tokens INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS audit_cache_write_tokens INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS audit_cost NUMERIC(20,10) NOT NULL DEFAULT 0;
