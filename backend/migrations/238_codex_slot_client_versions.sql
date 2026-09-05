-- Add per-slot Codex client version selection to reusable templates and
-- materialized account device slots. Existing rows inherit the global version.

ALTER TABLE account_codex_device_slots
    ADD COLUMN IF NOT EXISTS client_version_mode VARCHAR(20) NOT NULL DEFAULT 'inherit';
ALTER TABLE account_codex_device_slots
    ADD COLUMN IF NOT EXISTS client_version VARCHAR(64) NOT NULL DEFAULT '';

ALTER TABLE codex_identity_template_slots
    ADD COLUMN IF NOT EXISTS client_version_mode VARCHAR(20) NOT NULL DEFAULT 'inherit';
ALTER TABLE codex_identity_template_slots
    ADD COLUMN IF NOT EXISTS client_version VARCHAR(64) NOT NULL DEFAULT '';

ALTER TABLE account_codex_device_slots
    DROP CONSTRAINT IF EXISTS account_codex_slot_client_version_mode_check;
ALTER TABLE account_codex_device_slots
    ADD CONSTRAINT account_codex_slot_client_version_mode_check
    CHECK (client_version_mode IN ('inherit', 'pinned'));

ALTER TABLE account_codex_device_slots
    DROP CONSTRAINT IF EXISTS account_codex_slot_client_version_shape_check;
ALTER TABLE account_codex_device_slots
    ADD CONSTRAINT account_codex_slot_client_version_shape_check
    CHECK (
        (client_version_mode = 'inherit' AND client_version = '')
        OR
        (client_version_mode = 'pinned'
         AND client_version ~ '^[0-9]+(\.[0-9]+){1,3}(-[0-9A-Za-z.]+)?$')
    );

ALTER TABLE codex_identity_template_slots
    DROP CONSTRAINT IF EXISTS codex_identity_template_slot_client_version_mode_check;
ALTER TABLE codex_identity_template_slots
    ADD CONSTRAINT codex_identity_template_slot_client_version_mode_check
    CHECK (client_version_mode IN ('inherit', 'pinned'));

ALTER TABLE codex_identity_template_slots
    DROP CONSTRAINT IF EXISTS codex_identity_template_slot_client_version_shape_check;
ALTER TABLE codex_identity_template_slots
    ADD CONSTRAINT codex_identity_template_slot_client_version_shape_check
    CHECK (
        (client_version_mode = 'inherit' AND client_version = '')
        OR
        (client_version_mode = 'pinned'
         AND client_version ~ '^[0-9]+(\.[0-9]+){1,3}(-[0-9A-Za-z.]+)?$')
    );
