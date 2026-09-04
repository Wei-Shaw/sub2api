-- Expand Codex Profile identity from OS-only to (OS, canonical surface).
-- Existing rows have exactly one surface per OS and can be backfilled safely.

ALTER TABLE account_codex_device_bindings
    ADD COLUMN IF NOT EXISTS canonical_surface VARCHAR(20);

UPDATE account_codex_device_bindings AS bindings
SET canonical_surface = profiles.canonical_surface
FROM account_codex_device_slots AS slots,
     account_codex_profiles AS profiles
WHERE bindings.slot_id = slots.id
  AND slots.profile_id = profiles.id
  AND bindings.account_id = slots.account_id
  AND bindings.canonical_surface IS NULL;

ALTER TABLE account_codex_device_bindings
    ALTER COLUMN canonical_surface SET NOT NULL;

ALTER TABLE account_codex_device_bindings
    DROP CONSTRAINT IF EXISTS account_codex_device_bindings_account_id_api_key_id_os_class_key;
-- PostgreSQL truncates the generated 232 constraint name differently from a
-- later parsed identifier, so drop the actual catalog name as well.
ALTER TABLE account_codex_device_bindings
    DROP CONSTRAINT IF EXISTS account_codex_device_bindings_account_id_api_key_id_os_clas_key;
ALTER TABLE account_codex_device_bindings
    DROP CONSTRAINT IF EXISTS account_codex_binding_surface_check;
ALTER TABLE account_codex_device_bindings
    ADD CONSTRAINT account_codex_binding_surface_check
        CHECK (canonical_surface IN ('desktop', 'cli', 'sdk', 'third_party'));
ALTER TABLE account_codex_device_bindings
    DROP CONSTRAINT IF EXISTS account_codex_binding_profile_key;
ALTER TABLE account_codex_device_bindings
    ADD CONSTRAINT account_codex_binding_profile_key
        UNIQUE(account_id, api_key_id, os_class, canonical_surface);

ALTER TABLE account_codex_profiles
    DROP CONSTRAINT IF EXISTS account_codex_profiles_account_id_os_class_epoch_key;
ALTER TABLE account_codex_profiles
    DROP CONSTRAINT IF EXISTS account_codex_profile_os_epoch_key;
ALTER TABLE account_codex_profiles
    DROP CONSTRAINT IF EXISTS account_codex_profile_surface_epoch_key;
ALTER TABLE account_codex_profiles
    ADD CONSTRAINT account_codex_profile_surface_epoch_key
        UNIQUE(account_id, os_class, canonical_surface, epoch);

DROP INDEX IF EXISTS idx_account_codex_profiles_account;
CREATE INDEX idx_account_codex_profiles_account
    ON account_codex_profiles(account_id, os_class, canonical_surface, epoch);

DROP INDEX IF EXISTS idx_account_codex_bindings_api_key;
CREATE INDEX idx_account_codex_bindings_api_key
    ON account_codex_device_bindings(api_key_id, os_class, canonical_surface);

ALTER TABLE account_codex_identity_policies
    DROP CONSTRAINT IF EXISTS account_codex_identity_binding_scope_check;

UPDATE account_codex_identity_policies
SET binding_scope = 'api_key_os_surface', updated_at = NOW()
WHERE binding_scope = 'api_key_os';

ALTER TABLE account_codex_identity_policies
    ALTER COLUMN binding_scope SET DEFAULT 'api_key_os_surface';
ALTER TABLE account_codex_identity_policies
    ADD CONSTRAINT account_codex_identity_binding_scope_check
        CHECK (binding_scope = 'api_key_os_surface');

UPDATE accounts
SET codex_identity_policy = jsonb_set(
        codex_identity_policy,
        '{binding_scope}',
        '"api_key_os_surface"'::jsonb,
        TRUE
    )
WHERE COALESCE(codex_identity_policy->>'binding_scope', 'api_key_os') = 'api_key_os';
