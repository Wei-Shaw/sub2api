-- Data downgrade for rolling back to a binary that predates migrations 233/234.
-- This intentionally disables every OS Profile policy because an old binary
-- cannot represent two surfaces for one OS or a template assignment. Account
-- credentials, groups, proxies, usage and billing data are not changed.

BEGIN;

LOCK TABLE accounts,
           account_codex_identity_policies,
           account_codex_profiles,
           account_codex_device_slots,
           account_codex_device_bindings
IN SHARE ROW EXCLUSIVE MODE;

DELETE FROM account_codex_device_bindings;
DELETE FROM account_codex_device_slots;
DELETE FROM account_codex_profiles;

ALTER TABLE account_codex_identity_policies
    DROP CONSTRAINT IF EXISTS account_codex_identity_binding_scope_check;

UPDATE account_codex_identity_policies
SET mode='off',
    binding_scope='api_key_os',
    session_policy='{"mode":"conversation_isolated"}'::jsonb,
    affinity_ttl_seconds=3600,
    unsupported_policy='reject',
    version=version+1,
    updated_at=NOW();

UPDATE accounts
SET codex_identity_policy='{
      "mode":"off",
      "binding_scope":"api_key_os",
      "session_policy":{"mode":"conversation_isolated"},
      "affinity_ttl_seconds":3600,
      "unsupported_policy":"reject",
      "version":1
    }'::jsonb,
    updated_at=NOW();

-- Migration files are committed independently. If 233 succeeded but 234
-- failed, these columns do not exist yet; use dynamic SQL so the same
-- downgrade also handles that valid intermediate state.
DO $$
BEGIN
    IF (
        SELECT COUNT(*) = 2 FROM information_schema.columns
        WHERE table_schema='public'
          AND table_name='accounts'
          AND column_name IN (
              'codex_identity_template_id',
              'codex_identity_template_applied_revision'
          )
    ) THEN
        EXECUTE $sql$
            UPDATE accounts
            SET codex_identity_template_id=NULL,
                codex_identity_template_applied_revision=NULL
        $sql$;
    END IF;
END $$;

ALTER TABLE account_codex_identity_policies
    ALTER COLUMN binding_scope SET DEFAULT 'api_key_os';
ALTER TABLE account_codex_identity_policies
    ADD CONSTRAINT account_codex_identity_binding_scope_check
        CHECK (binding_scope = 'api_key_os');

ALTER TABLE account_codex_device_bindings
    DROP CONSTRAINT IF EXISTS account_codex_binding_profile_key;
ALTER TABLE account_codex_device_bindings
    DROP CONSTRAINT IF EXISTS account_codex_device_bindings_account_id_api_key_id_os_class_key;
ALTER TABLE account_codex_device_bindings
    DROP CONSTRAINT IF EXISTS account_codex_device_bindings_account_id_api_key_id_os_clas_key;
ALTER TABLE account_codex_device_bindings
    DROP CONSTRAINT IF EXISTS account_codex_binding_surface_check;
DROP INDEX IF EXISTS idx_account_codex_bindings_api_key;
ALTER TABLE account_codex_device_bindings
    DROP COLUMN IF EXISTS canonical_surface;
ALTER TABLE account_codex_device_bindings
    ADD CONSTRAINT account_codex_binding_profile_key
        UNIQUE(account_id, api_key_id, os_class);
CREATE INDEX idx_account_codex_bindings_api_key
    ON account_codex_device_bindings(api_key_id, os_class);

ALTER TABLE account_codex_profiles
    DROP CONSTRAINT IF EXISTS account_codex_profile_surface_epoch_key;
ALTER TABLE account_codex_profiles
    DROP CONSTRAINT IF EXISTS account_codex_profiles_account_id_os_class_epoch_key;
ALTER TABLE account_codex_profiles
    DROP CONSTRAINT IF EXISTS account_codex_profile_os_epoch_key;
DROP INDEX IF EXISTS idx_account_codex_profiles_account;
ALTER TABLE account_codex_profiles
    ADD CONSTRAINT account_codex_profiles_account_id_os_class_epoch_key
        UNIQUE(account_id, os_class, epoch);
CREATE INDEX idx_account_codex_profiles_account
    ON account_codex_profiles(account_id, os_class, epoch);

ALTER TABLE accounts
    DROP CONSTRAINT IF EXISTS accounts_codex_identity_template_fk;
ALTER TABLE accounts
    DROP CONSTRAINT IF EXISTS accounts_codex_identity_template_revision_check;
DROP INDEX IF EXISTS idx_accounts_codex_identity_template;
ALTER TABLE accounts
    DROP COLUMN IF EXISTS codex_identity_template_applied_revision;
ALTER TABLE accounts
    DROP COLUMN IF EXISTS codex_identity_template_id;

DROP TABLE IF EXISTS codex_identity_template_slots;
DROP TABLE IF EXISTS codex_identity_template_profiles;
DROP TABLE IF EXISTS codex_identity_templates;

DO $$
BEGIN
    IF to_regclass('public.schema_migrations') IS NOT NULL THEN
        EXECUTE $sql$
            DELETE FROM schema_migrations
            WHERE filename IN (
                '233_codex_profile_surface_identity.sql',
                '234_codex_identity_templates.sql'
            )
        $sql$;
    END IF;
END $$;

COMMIT;
