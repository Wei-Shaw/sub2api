ALTER TABLE usage_logs
    ADD COLUMN IF NOT EXISTS routing_target_group VARCHAR(32),
    ADD COLUMN IF NOT EXISTS routing_schedule_layer VARCHAR(64),
    ADD COLUMN IF NOT EXISTS routing_selected_account_id BIGINT,
    ADD COLUMN IF NOT EXISTS routing_selected_account_name VARCHAR(255),
    ADD COLUMN IF NOT EXISTS routing_effective_model VARCHAR(100),
    ADD COLUMN IF NOT EXISTS routing_failover_count INTEGER,
    ADD COLUMN IF NOT EXISTS routing_failover_final_reason VARCHAR(128);

ALTER TABLE ops_error_logs
    ADD COLUMN IF NOT EXISTS routing_target_group VARCHAR(32),
    ADD COLUMN IF NOT EXISTS routing_schedule_layer VARCHAR(64),
    ADD COLUMN IF NOT EXISTS routing_selected_account_id BIGINT,
    ADD COLUMN IF NOT EXISTS routing_selected_account_name VARCHAR(255),
    ADD COLUMN IF NOT EXISTS routing_requested_model VARCHAR(100),
    ADD COLUMN IF NOT EXISTS routing_effective_model VARCHAR(100),
    ADD COLUMN IF NOT EXISTS routing_failover_count INTEGER,
    ADD COLUMN IF NOT EXISTS routing_failover_final_reason VARCHAR(128);

CREATE INDEX IF NOT EXISTS idx_usage_logs_created_routing_target_group
    ON usage_logs (created_at, routing_target_group);

CREATE INDEX IF NOT EXISTS idx_usage_logs_created_routing_schedule_layer
    ON usage_logs (created_at, routing_schedule_layer);

CREATE INDEX IF NOT EXISTS idx_ops_error_logs_created_routing_target_group
    ON ops_error_logs (created_at, routing_target_group);

CREATE INDEX IF NOT EXISTS idx_ops_error_logs_created_routing_schedule_layer
    ON ops_error_logs (created_at, routing_schedule_layer);

COMMENT ON COLUMN usage_logs.routing_target_group IS 'Final OpenAI target group chosen for routing: active or exhausted.';
COMMENT ON COLUMN usage_logs.routing_schedule_layer IS 'Final scheduler layer that selected the account, e.g. previous_response_id, session_hash, load_balance.';
COMMENT ON COLUMN usage_logs.routing_selected_account_id IS 'Final selected account id for OpenAI routing snapshot.';
COMMENT ON COLUMN usage_logs.routing_selected_account_name IS 'Final selected account name for OpenAI routing snapshot.';
COMMENT ON COLUMN usage_logs.routing_effective_model IS 'Effective upstream-facing model after alias stripping / mapping in the OpenAI routing snapshot.';
COMMENT ON COLUMN usage_logs.routing_failover_count IS 'Number of failover switches observed before the request completed.';
COMMENT ON COLUMN usage_logs.routing_failover_final_reason IS 'Last recorded failover reason in the OpenAI routing snapshot.';

COMMENT ON COLUMN ops_error_logs.routing_target_group IS 'Final OpenAI target group chosen for routing: active or exhausted.';
COMMENT ON COLUMN ops_error_logs.routing_schedule_layer IS 'Final scheduler layer that selected the account, e.g. previous_response_id, session_hash, load_balance.';
COMMENT ON COLUMN ops_error_logs.routing_selected_account_id IS 'Final selected account id for OpenAI routing snapshot.';
COMMENT ON COLUMN ops_error_logs.routing_selected_account_name IS 'Final selected account name for OpenAI routing snapshot.';
COMMENT ON COLUMN ops_error_logs.routing_requested_model IS 'Client-requested model recorded in the OpenAI routing snapshot.';
COMMENT ON COLUMN ops_error_logs.routing_effective_model IS 'Effective upstream-facing model after alias stripping / mapping in the OpenAI routing snapshot.';
COMMENT ON COLUMN ops_error_logs.routing_failover_count IS 'Number of failover switches observed before the request failed.';
COMMENT ON COLUMN ops_error_logs.routing_failover_final_reason IS 'Last recorded failover reason in the OpenAI routing snapshot.';
