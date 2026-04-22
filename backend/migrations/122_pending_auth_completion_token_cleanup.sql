UPDATE pending_auth_sessions
SET
    local_flow_state = jsonb_set(
        local_flow_state,
        '{completion_response}',
        ((local_flow_state -> 'completion_response') - 'access_token' - 'refresh_token' - 'expires_in' - 'token_type'),
        true
    )
WHERE jsonb_typeof(local_flow_state -> 'completion_response') = 'object'
  AND (
      (local_flow_state -> 'completion_response') ? 'access_token'
      OR (local_flow_state -> 'completion_response') ? 'refresh_token'
      OR (local_flow_state -> 'completion_response') ? 'expires_in'
      OR (local_flow_state -> 'completion_response') ? 'token_type'
  );
