ALTER TABLE api_keys
    ADD COLUMN IF NOT EXISTS is_evaluation BOOLEAN NOT NULL DEFAULT FALSE;

CREATE TABLE IF NOT EXISTS evaluation_route_evidence (
    route_trace_id VARCHAR(64) PRIMARY KEY,
    evaluation_run_id UUID NOT NULL,
    sample_id UUID NOT NULL,
    api_key_id BIGINT NOT NULL REFERENCES api_keys(id),
    request_id VARCHAR(128),
    requested_model VARCHAR(200) NOT NULL,
    resolved_model VARCHAR(200),
    route_profile_version VARCHAR(100) NOT NULL,
    provider VARCHAR(32),
    channel_ref VARCHAR(64),
    account_pool_ref VARCHAR(64),
    region VARCHAR(64) NOT NULL,
    attempts INT NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    fallback_chain JSONB NOT NULL DEFAULT '[]'::jsonb,
    finish_reason VARCHAR(64),
    input_tokens INT CHECK (input_tokens >= 0),
    output_tokens INT CHECK (output_tokens >= 0),
    ttft_ms INT CHECK (ttft_ms >= 0),
    latency_ms INT CHECK (latency_ms >= 0),
    billed_amount NUMERIC(20,8),
    transport_status VARCHAR(24) NOT NULL DEFAULT 'started'
        CHECK (transport_status IN (
            'started',
            'succeeded',
            'upstream_failed',
            'protocol_failed',
            'client_cancelled',
            'gateway_failed'
        )),
    error_code VARCHAR(100),
    started_at TIMESTAMPTZ NOT NULL,
    finished_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_eval_route_evidence_run_sample
    ON evaluation_route_evidence (evaluation_run_id, sample_id);

CREATE INDEX IF NOT EXISTS idx_eval_route_evidence_model_finished
    ON evaluation_route_evidence (requested_model, finished_at DESC);
