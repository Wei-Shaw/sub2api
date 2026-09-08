-- Preserve explicit direct routing for nested Codex proxy overrides and add
-- indexes required by proxy-reference and draining-slot lifecycle queries.

ALTER TABLE account_codex_profiles
    ADD COLUMN IF NOT EXISTS proxy_mode VARCHAR(20);

UPDATE account_codex_profiles
SET proxy_mode = CASE
    WHEN proxy_mode = 'direct' AND proxy_id IS NULL THEN 'direct'
    WHEN proxy_id IS NOT NULL THEN 'proxy'
    ELSE 'inherit'
END;

ALTER TABLE account_codex_profiles
    ALTER COLUMN proxy_mode SET DEFAULT 'inherit',
    ALTER COLUMN proxy_mode SET NOT NULL;

ALTER TABLE account_codex_profiles
    DROP CONSTRAINT IF EXISTS account_codex_profile_proxy_mode_check;
ALTER TABLE account_codex_profiles
    ADD CONSTRAINT account_codex_profile_proxy_mode_check
    CHECK (proxy_mode IN ('inherit', 'proxy', 'direct'));

ALTER TABLE account_codex_profiles
    DROP CONSTRAINT IF EXISTS account_codex_profile_proxy_shape_check;
ALTER TABLE account_codex_profiles
    ADD CONSTRAINT account_codex_profile_proxy_shape_check
    CHECK (
        (proxy_mode = 'proxy' AND proxy_id IS NOT NULL)
        OR (proxy_mode IN ('inherit', 'direct') AND proxy_id IS NULL)
    );

ALTER TABLE account_codex_device_slots
    ADD COLUMN IF NOT EXISTS proxy_mode VARCHAR(20);

UPDATE account_codex_device_slots
SET proxy_mode = CASE
    WHEN proxy_mode = 'direct' AND proxy_id IS NULL THEN 'direct'
    WHEN proxy_id IS NOT NULL THEN 'proxy'
    ELSE 'inherit'
END;

ALTER TABLE account_codex_device_slots
    ALTER COLUMN proxy_mode SET DEFAULT 'inherit',
    ALTER COLUMN proxy_mode SET NOT NULL;

ALTER TABLE account_codex_device_slots
    DROP CONSTRAINT IF EXISTS account_codex_slot_proxy_mode_check;
ALTER TABLE account_codex_device_slots
    ADD CONSTRAINT account_codex_slot_proxy_mode_check
    CHECK (proxy_mode IN ('inherit', 'proxy', 'direct'));

ALTER TABLE account_codex_device_slots
    DROP CONSTRAINT IF EXISTS account_codex_slot_proxy_shape_check;
ALTER TABLE account_codex_device_slots
    ADD CONSTRAINT account_codex_slot_proxy_shape_check
    CHECK (
        (proxy_mode = 'proxy' AND proxy_id IS NOT NULL)
        OR (proxy_mode IN ('inherit', 'direct') AND proxy_id IS NULL)
    );

-- Agent Identity is a distinct authentication mode. A stale bearer token must
-- not make an invalid runtime/private-key pair schedulable.
UPDATE accounts
SET provisioning_state = 'pending',
    schedulable = FALSE,
    updated_at = NOW()
WHERE provisioning_state = 'active'
  AND COALESCE(codex_identity_policy->>'mode', 'off') = 'os_profile_device_pool'
  AND LOWER(COALESCE(credentials->>'auth_mode', '')) = LOWER('agentIdentity')
  AND (
      NULLIF(BTRIM(credentials->>'agent_runtime_id'), '') IS NULL
      OR NULLIF(BTRIM(credentials->>'agent_private_key'), '') IS NULL
  );

ALTER TABLE accounts
    DROP CONSTRAINT IF EXISTS accounts_codex_identity_active_credentials_check;
ALTER TABLE accounts
    ADD CONSTRAINT accounts_codex_identity_active_credentials_check
    CHECK (
        provisioning_state <> 'active'
        OR COALESCE(codex_identity_policy->>'mode', 'off') <> 'os_profile_device_pool'
        OR (
            platform = 'openai'
            AND type = 'oauth'
            AND CASE
                WHEN LOWER(COALESCE(credentials->>'auth_mode', '')) = LOWER('agentIdentity') THEN
                    NULLIF(BTRIM(credentials->>'agent_runtime_id'), '') IS NOT NULL
                    AND NULLIF(BTRIM(credentials->>'agent_private_key'), '') IS NOT NULL
                ELSE
                    NULLIF(BTRIM(credentials->>'access_token'), '') IS NOT NULL
                    OR NULLIF(BTRIM(credentials->>'refresh_token'), '') IS NOT NULL
            END
        )
    );

CREATE INDEX IF NOT EXISTS idx_account_codex_profiles_proxy
    ON account_codex_profiles(proxy_id)
    WHERE proxy_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_account_codex_slots_proxy
    ON account_codex_device_slots(proxy_id)
    WHERE proxy_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_account_codex_bindings_slot
    ON account_codex_device_bindings(slot_id);
