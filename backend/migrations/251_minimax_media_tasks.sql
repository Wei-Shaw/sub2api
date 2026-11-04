-- Durable receipts survive Redis loss and key/account removal.
CREATE TABLE minimax_media_tasks (
 id VARCHAR(64) PRIMARY KEY,
 user_id BIGINT NOT NULL, api_key_id BIGINT NOT NULL, group_id BIGINT NOT NULL,
 account_id BIGINT NOT NULL, subscription_id BIGINT, account_type VARCHAR(32) NOT NULL,
 upstream_id VARCHAR(128), origin VARCHAR(256) NOT NULL,
 kind VARCHAR(16) NOT NULL CHECK(kind IN ('speech','video')),
 idempotency_key VARCHAR(128) NOT NULL, request_hash VARCHAR(64) NOT NULL,
 snapshot JSONB NOT NULL, reserved_usd NUMERIC(20,8) NOT NULL CHECK(reserved_usd>=0),
 reservation_windows JSONB NOT NULL DEFAULT '{}',
 status VARCHAR(32) NOT NULL DEFAULT 'submitting',
 billing_state VARCHAR(16) NOT NULL DEFAULT 'reserved' CHECK(billing_state IN ('reserved','settled','released')),
 bill JSONB, result JSONB,
 created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(), updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
 next_poll_at TIMESTAMPTZ NOT NULL DEFAULT NOW()+INTERVAL '10 seconds',
 UNIQUE(api_key_id,idempotency_key), UNIQUE(account_id,upstream_id)
);
CREATE INDEX minimax_media_tasks_owner ON minimax_media_tasks(user_id,api_key_id,created_at DESC);
CREATE INDEX minimax_media_tasks_poll ON minimax_media_tasks(next_poll_at) WHERE billing_state='reserved';
ALTER TABLE composite_model_routes DROP CONSTRAINT composite_model_routes_endpoint_check;
ALTER TABLE composite_model_routes ADD CONSTRAINT composite_model_routes_endpoint_check
 CHECK(endpoint IN ('any','messages','count_tokens','responses','chat_completions','embeddings','images','gemini','speech','video'));
