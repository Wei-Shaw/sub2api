-- Existing users default to an indefinite lock. Existing key locks have no deadline.
-- Preexisting rows satisfy these constraints by construction. Enforce new writes without a table scan.
ALTER TABLE users
    ADD COLUMN moderation_key_lock_minutes INTEGER NOT NULL DEFAULT 0,
    ADD CONSTRAINT users_moderation_key_lock_duration_check
        CHECK (moderation_key_lock_minutes = 0 OR
               (moderation_key_lock_minutes > 0 AND moderation_penalty_scope = 'api_key')) NOT VALID;
ALTER TABLE api_keys
    ADD COLUMN moderation_locked_until TIMESTAMPTZ,
    ADD CONSTRAINT api_keys_moderation_lock_deadline_check
        CHECK (moderation_locked_until IS NULL OR
               (moderation_locked_at IS NOT NULL AND moderation_locked_until > moderation_locked_at)) NOT VALID;
ALTER TABLE content_moderation_logs ADD COLUMN penalty_locked_until TIMESTAMPTZ;
