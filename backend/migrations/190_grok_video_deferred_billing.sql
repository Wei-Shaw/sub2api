-- Migration 190: persist Grok asynchronous video requests until their terminal status is
-- observed. Billing is applied only after a successful status lookup.

CREATE TABLE IF NOT EXISTS grok_video_settlements (
    id BIGSERIAL PRIMARY KEY,
    request_id VARCHAR(255) NOT NULL,
    request_fingerprint VARCHAR(64) NOT NULL,
    user_id BIGINT NOT NULL,
    api_key_id BIGINT NOT NULL,
    group_id BIGINT,
    account_id BIGINT NOT NULL,
    account_type VARCHAR(32) NOT NULL,
    subscription_id BIGINT,
    requested_model VARCHAR(100) NOT NULL,
    billing_model VARCHAR(100) NOT NULL,
    upstream_model VARCHAR(100) NOT NULL DEFAULT '',
    input_tokens INTEGER NOT NULL DEFAULT 0,
    output_tokens INTEGER NOT NULL DEFAULT 0,
    cache_creation_tokens INTEGER NOT NULL DEFAULT 0,
    cache_read_tokens INTEGER NOT NULL DEFAULT 0,
    image_input_tokens INTEGER NOT NULL DEFAULT 0,
    image_output_tokens INTEGER NOT NULL DEFAULT 0,
    video_resolution VARCHAR(10) NOT NULL,
    video_duration_seconds INTEGER NOT NULL,
    request_duration_ms BIGINT NOT NULL DEFAULT 0,
    request_payload_hash VARCHAR(64) NOT NULL DEFAULT '',
    inbound_endpoint VARCHAR(255) NOT NULL DEFAULT '',
    upstream_endpoint VARCHAR(255) NOT NULL DEFAULT '',
    user_agent VARCHAR(512) NOT NULL DEFAULT '',
    ip_address VARCHAR(45) NOT NULL DEFAULT '',
    session_id VARCHAR(255) NOT NULL DEFAULT '',
    quota_platform VARCHAR(32) NOT NULL DEFAULT '',
    channel_id BIGINT NOT NULL DEFAULT 0,
    channel_mapped_model VARCHAR(100) NOT NULL DEFAULT '',
    billing_model_source VARCHAR(32) NOT NULL DEFAULT '',
    model_mapping_chain VARCHAR(500) NOT NULL DEFAULT '',
    pricing_snapshot_version INTEGER NOT NULL,
    pricing_basis VARCHAR(24) NOT NULL,
    billing_mode VARCHAR(16) NOT NULL,
    billing_type SMALLINT NOT NULL,
    input_cost NUMERIC(20, 10) NOT NULL DEFAULT 0,
    image_input_cost NUMERIC(20, 10) NOT NULL DEFAULT 0,
    output_cost NUMERIC(20, 10) NOT NULL DEFAULT 0,
    image_output_cost NUMERIC(20, 10) NOT NULL DEFAULT 0,
    cache_creation_cost NUMERIC(20, 10) NOT NULL DEFAULT 0,
    cache_read_cost NUMERIC(20, 10) NOT NULL DEFAULT 0,
    total_cost NUMERIC(20, 10) NOT NULL DEFAULT 0,
    actual_cost NUMERIC(20, 10) NOT NULL DEFAULT 0,
    rate_multiplier NUMERIC(20, 10) NOT NULL DEFAULT 0,
    account_rate_multiplier NUMERIC(20, 10) NOT NULL DEFAULT 0,
    long_context_billing_applied BOOLEAN NOT NULL DEFAULT FALSE,
    account_stats_cost NUMERIC(20, 10),
    status VARCHAR(16) NOT NULL DEFAULT 'pending',
    terminal_at TIMESTAMPTZ,
    settled_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT grok_video_settlements_status_check
        CHECK (status IN ('pending', 'settled', 'failed', 'expired', 'cancelled')),
    CONSTRAINT grok_video_settlements_duration_check
        CHECK (video_duration_seconds > 0),
    CONSTRAINT grok_video_settlements_pricing_version_check
        CHECK (pricing_snapshot_version > 0),
    CONSTRAINT grok_video_settlements_pricing_basis_check
        CHECK (pricing_basis IN ('video_second', 'fixed_request', 'token')),
    CONSTRAINT grok_video_settlements_billing_type_check
        CHECK (billing_type IN (0, 1)),
    CONSTRAINT grok_video_settlements_pricing_values_check
        CHECK (
            input_cost >= 0 AND image_input_cost >= 0 AND output_cost >= 0 AND
            image_output_cost >= 0 AND cache_creation_cost >= 0 AND cache_read_cost >= 0 AND
            total_cost >= 0 AND actual_cost >= 0 AND rate_multiplier >= 0 AND
            account_rate_multiplier >= 0 AND (account_stats_cost IS NULL OR account_stats_cost >= 0)
    )
);

-- A pre-merge build may already have created this table from the original
-- migration number. CREATE TABLE IF NOT EXISTS does not add new columns.
ALTER TABLE grok_video_settlements
    ADD COLUMN IF NOT EXISTS session_id VARCHAR(255) NOT NULL DEFAULT '';

CREATE UNIQUE INDEX IF NOT EXISTS grok_video_settlements_request_api_key_uq
    ON grok_video_settlements (request_id, api_key_id);

CREATE INDEX IF NOT EXISTS grok_video_settlements_owner_idx
    ON grok_video_settlements (user_id, api_key_id, created_at DESC);

CREATE INDEX IF NOT EXISTS grok_video_settlements_pending_idx
    ON grok_video_settlements (created_at)
    WHERE status = 'pending';

COMMENT ON TABLE grok_video_settlements IS
    'Pending Grok asynchronous video billing snapshots; settled only after status=done';
