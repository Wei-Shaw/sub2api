-- Based on Randark's passive history model (#5878); narrowed to ordinary
-- OpenAI OAuth accounts and backed by a durable observation journal.
-- Existing usage rows with NULL reference prices remain unknown.
CREATE TABLE IF NOT EXISTS account_quota_observations (
 id BIGSERIAL PRIMARY KEY,
 account_id BIGINT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
 observed_at TIMESTAMPTZ NOT NULL,
 payload JSONB NOT NULL,
 processed_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_account_quota_observations_pending
 ON account_quota_observations(account_id,id) WHERE processed_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_account_quota_observations_ready
 ON account_quota_observations(id) INCLUDE(account_id,observed_at) WHERE processed_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_account_quota_observations_reset
 ON account_quota_observations(account_id,observed_at) WHERE payload ? 'codex_history_reset_at';
CREATE INDEX IF NOT EXISTS idx_account_quota_observations_retention
 ON account_quota_observations(observed_at) WHERE processed_at IS NOT NULL;

CREATE TABLE IF NOT EXISTS account_window_usage_histories (
 id BIGSERIAL PRIMARY KEY,
 account_id BIGINT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
 window_type VARCHAR(32) NOT NULL CHECK (window_type IN ('5h','7d')),
 window_start TIMESTAMPTZ NOT NULL,
 window_end TIMESTAMPTZ NOT NULL,
 reset_at TIMESTAMPTZ NOT NULL,
 duration_minutes INT NOT NULL,
 first_observed_at TIMESTAMPTZ NOT NULL,
 last_sample_at TIMESTAMPTZ,
 last_observation_id BIGINT NOT NULL DEFAULT 0,
 peak_used_percent DOUBLE PRECISION NOT NULL DEFAULT 0,
 last_used_percent DOUBLE PRECISION NOT NULL DEFAULT 0,
 sample_count INT NOT NULL DEFAULT 0,
 requests BIGINT NOT NULL DEFAULT 0,
 tokens_total BIGINT NOT NULL DEFAULT 0,
 api_reference_cost NUMERIC(20,10),
 priced_requests BIGINT NOT NULL DEFAULT 0,
 missing_pricing_requests BIGINT NOT NULL DEFAULT 0,
 estimated_reference_limit NUMERIC(20,10),
 estimate_reference_cost NUMERIC(20,10),
 estimate_used_percent DOUBLE PRECISION,
 estimate_observed_at TIMESTAMPTZ,
 quality_flags JSONB NOT NULL DEFAULT '[]',
 end_reason VARCHAR(32),
 finalized_at TIMESTAMPTZ,
 stats_finalized_at TIMESTAMPTZ,
 created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
 CONSTRAINT ck_account_window_usage_range CHECK (window_end>window_start)
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_account_window_usage_open
 ON account_window_usage_histories(account_id,window_type) WHERE finalized_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_account_window_usage_history
 ON account_window_usage_histories(account_id,window_type,window_end DESC);
CREATE INDEX IF NOT EXISTS idx_account_window_usage_reconcile
 ON account_window_usage_histories(window_end,id) WHERE stats_finalized_at IS NULL;
COMMENT ON TABLE account_quota_observations IS 'Minimal Codex observations journaled with accounts.extra updates; acknowledged transactionally after history processing';
COMMENT ON COLUMN account_window_usage_histories.api_reference_cost IS 'Sum of available local immutable API reference amounts, not an upstream monetary quota';
COMMENT ON COLUMN account_window_usage_histories.estimated_reference_limit IS 'Last usable local API equivalent estimate; numerator, utilization and observation time stored separately';
