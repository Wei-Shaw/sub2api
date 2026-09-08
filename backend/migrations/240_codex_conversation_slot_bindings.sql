-- Keep the existing API-key preference (empty hash) and pin individual
-- conversations without moving other conversations belonging to that key.
ALTER TABLE account_codex_device_bindings
    ADD COLUMN conversation_hash VARCHAR(64) NOT NULL DEFAULT '';
ALTER TABLE account_codex_device_bindings
    ADD CONSTRAINT account_codex_binding_conversation_hash_check
        CHECK (conversation_hash = '' OR conversation_hash ~ '^[0-9a-f]{64}$');
ALTER TABLE account_codex_device_bindings
    DROP CONSTRAINT account_codex_binding_profile_key;
ALTER TABLE account_codex_device_bindings
    ADD CONSTRAINT account_codex_binding_profile_key
        UNIQUE(account_id, api_key_id, os_class, canonical_surface, conversation_hash);
