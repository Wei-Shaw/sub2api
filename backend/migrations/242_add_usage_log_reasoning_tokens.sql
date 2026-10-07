-- Observed breakdown only: output_tokens and billing already include reasoning.
ALTER TABLE usage_logs ADD COLUMN IF NOT EXISTS reasoning_tokens INTEGER
    CHECK (reasoning_tokens >= 0);
COMMENT ON COLUMN usage_logs.reasoning_tokens IS 'Upstream-reported reasoning tokens; NULL means unavailable, not zero';
