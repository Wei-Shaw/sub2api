ALTER TABLE usage_logs
    ADD COLUMN IF NOT EXISTS sticky_session_source VARCHAR(64),
    ADD COLUMN IF NOT EXISTS sticky_session_hash_present BOOLEAN,
    ADD COLUMN IF NOT EXISTS sticky_eval_result VARCHAR(64),
    ADD COLUMN IF NOT EXISTS sticky_selected_account_changed BOOLEAN,
    ADD COLUMN IF NOT EXISTS sticky_parent_session_present BOOLEAN,
    ADD COLUMN IF NOT EXISTS sticky_parent_session_key VARCHAR(128);

ALTER TABLE ops_error_logs
    ADD COLUMN IF NOT EXISTS sticky_session_source VARCHAR(64),
    ADD COLUMN IF NOT EXISTS sticky_session_hash_present BOOLEAN,
    ADD COLUMN IF NOT EXISTS sticky_eval_result VARCHAR(64),
    ADD COLUMN IF NOT EXISTS sticky_selected_account_changed BOOLEAN,
    ADD COLUMN IF NOT EXISTS sticky_parent_session_present BOOLEAN,
    ADD COLUMN IF NOT EXISTS sticky_parent_session_key VARCHAR(128);

COMMENT ON COLUMN usage_logs.sticky_session_source IS 'Sticky session signal source in the routing snapshot, e.g. header_x_session_affinity or prompt_cache_key.';
COMMENT ON COLUMN usage_logs.sticky_session_hash_present IS 'Whether sticky evaluation observed a non-empty session hash.';
COMMENT ON COLUMN usage_logs.sticky_eval_result IS 'Sticky evaluation result captured in the routing snapshot.';
COMMENT ON COLUMN usage_logs.sticky_selected_account_changed IS 'Whether sticky evaluation switched away from the originally bound account.';
COMMENT ON COLUMN usage_logs.sticky_parent_session_present IS 'Whether sticky evaluation observed a parent session / previous_response_id signal.';
COMMENT ON COLUMN usage_logs.sticky_parent_session_key IS 'Parent session key captured in sticky evaluation, usually previous_response_id.';

COMMENT ON COLUMN ops_error_logs.sticky_session_source IS 'Sticky session signal source in the routing snapshot, e.g. header_x_session_affinity or prompt_cache_key.';
COMMENT ON COLUMN ops_error_logs.sticky_session_hash_present IS 'Whether sticky evaluation observed a non-empty session hash.';
COMMENT ON COLUMN ops_error_logs.sticky_eval_result IS 'Sticky evaluation result captured in the routing snapshot.';
COMMENT ON COLUMN ops_error_logs.sticky_selected_account_changed IS 'Whether sticky evaluation switched away from the originally bound account.';
COMMENT ON COLUMN ops_error_logs.sticky_parent_session_present IS 'Whether sticky evaluation observed a parent session / previous_response_id signal.';
COMMENT ON COLUMN ops_error_logs.sticky_parent_session_key IS 'Parent session key captured in sticky evaluation, usually previous_response_id.';
