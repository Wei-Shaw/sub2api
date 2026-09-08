-- Allow a shared device identity to serve concurrent requests.
-- Keep explicit existing limits, including 1. Zero adds no slot-specific cap.
ALTER TABLE account_codex_identity_policies
    DROP CONSTRAINT account_codex_identity_session_policy_check;
ALTER TABLE account_codex_identity_policies
    ADD CONSTRAINT account_codex_identity_session_policy_check CHECK (
        jsonb_typeof(session_policy) = 'object'
        AND COALESCE(session_policy->>'mode', '') IN
            ('conversation_isolated', 'api_key_shared', 'session_pool', 'device_shared')
        AND (COALESCE(session_policy->>'mode', '') <> 'device_shared' OR (
            COALESCE((session_policy->>'max_active_conversations_per_slot')::INTEGER, 0) BETWEEN 0 AND 1000
            AND COALESCE((session_policy->>'disable_cross_key_continuation')::BOOLEAN, FALSE) = TRUE
        ))
    );

ALTER TABLE codex_identity_templates
    DROP CONSTRAINT codex_identity_template_session_policy_check;
ALTER TABLE codex_identity_templates
    ADD CONSTRAINT codex_identity_template_session_policy_check CHECK (
        jsonb_typeof(session_policy) = 'object'
        AND COALESCE(session_policy->>'mode', '') IN
            ('conversation_isolated', 'api_key_shared', 'session_pool', 'device_shared')
        AND (COALESCE(session_policy->>'mode', '') <> 'device_shared' OR (
            COALESCE((session_policy->>'max_active_conversations_per_slot')::INTEGER, 0) BETWEEN 0 AND 1000
            AND COALESCE((session_policy->>'disable_cross_key_continuation')::BOOLEAN, FALSE) = TRUE
        ))
    );
