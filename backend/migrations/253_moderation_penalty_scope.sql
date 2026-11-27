-- Additive migration: legacy log rows and users keep their original semantics.
ALTER TABLE users
    ADD COLUMN moderation_penalty_scope VARCHAR(16) NOT NULL DEFAULT 'user'
        CHECK (moderation_penalty_scope IN ('user','api_key')),
    ADD COLUMN moderation_scope_changed_at TIMESTAMPTZ;
ALTER TABLE api_keys
    ADD COLUMN moderation_locked_at TIMESTAMPTZ,
    ADD COLUMN moderation_last_unlocked_at TIMESTAMPTZ;
ALTER TABLE content_moderation_logs
    ADD COLUMN event_id UUID,
    ADD COLUMN occurred_at TIMESTAMPTZ,
    ADD COLUMN penalty_scope VARCHAR(16) CHECK (penalty_scope IN ('user','api_key')),
    ADD COLUMN penalty_action VARCHAR(24) CHECK (penalty_action IN ('user_disabled','api_key_locked'));
-- No credentials or personal data, only object identifiers and transitions.
CREATE TABLE moderation_admin_actions (
    id BIGSERIAL PRIMARY KEY, actor_id BIGINT NOT NULL, user_id BIGINT NOT NULL, api_key_id BIGINT,
    action VARCHAR(24) NOT NULL, old_value VARCHAR(16) NOT NULL, new_value VARCHAR(16) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);
