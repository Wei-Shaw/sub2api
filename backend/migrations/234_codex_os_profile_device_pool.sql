-- Codex OS Profile device pool provisioning boundary.
-- Existing accounts retain their current behavior; new provisioning-aware
-- writes explicitly choose pending or active after complete validation.

ALTER TABLE accounts
    ADD COLUMN IF NOT EXISTS provisioning_state VARCHAR(20) NOT NULL DEFAULT 'active';

ALTER TABLE accounts
    ADD COLUMN IF NOT EXISTS codex_identity_policy JSONB NOT NULL DEFAULT
    '{"mode":"off","session_policy":{"mode":"conversation_isolated"},"affinity_ttl_seconds":3600}'::jsonb;

UPDATE accounts
SET provisioning_state = 'active'
WHERE provisioning_state IS NULL OR provisioning_state NOT IN ('pending', 'active');

ALTER TABLE accounts ALTER COLUMN provisioning_state SET DEFAULT 'pending';

UPDATE accounts
SET codex_identity_policy =
    '{"mode":"off","session_policy":{"mode":"conversation_isolated"},"affinity_ttl_seconds":3600}'::jsonb
WHERE codex_identity_policy IS NULL OR jsonb_typeof(codex_identity_policy) <> 'object';

ALTER TABLE accounts DROP CONSTRAINT IF EXISTS accounts_provisioning_state_check;
ALTER TABLE accounts
    ADD CONSTRAINT accounts_provisioning_state_check
    CHECK (provisioning_state IN ('pending', 'active'));

CREATE OR REPLACE FUNCTION enforce_pending_account_unschedulable()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.provisioning_state <> 'active' THEN
        NEW.schedulable := FALSE;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_accounts_pending_unschedulable ON accounts;
CREATE TRIGGER trg_accounts_pending_unschedulable
BEFORE INSERT OR UPDATE OF provisioning_state, schedulable ON accounts
FOR EACH ROW EXECUTE FUNCTION enforce_pending_account_unschedulable();

CREATE OR REPLACE FUNCTION enforce_codex_identity_shadow_exclusion()
RETURNS TRIGGER AS $$
DECLARE
    parent_identity_mode TEXT;
BEGIN
    IF NEW.parent_account_id IS NOT NULL AND NEW.quota_dimension = 'spark' AND NEW.deleted_at IS NULL THEN
        IF COALESCE(NEW.codex_identity_policy->>'mode', 'off') = 'os_profile_device_pool' THEN
            RAISE EXCEPTION 'Spark shadow cannot enable Codex OS profile device pool'
                USING ERRCODE = '23514';
        END IF;
        SELECT COALESCE(codex_identity_policy->>'mode', 'off')
        INTO parent_identity_mode
        FROM accounts
        WHERE id = NEW.parent_account_id AND deleted_at IS NULL
        FOR UPDATE;
        IF parent_identity_mode = 'os_profile_device_pool' THEN
            RAISE EXCEPTION 'Spark shadow is incompatible with parent Codex OS profile device pool'
                USING ERRCODE = '23514';
        END IF;
    END IF;

    IF COALESCE(NEW.codex_identity_policy->>'mode', 'off') = 'os_profile_device_pool'
       AND EXISTS (
           SELECT 1 FROM accounts AS shadow
           WHERE shadow.parent_account_id = NEW.id
             AND shadow.quota_dimension = 'spark'
             AND shadow.deleted_at IS NULL
       ) THEN
        RAISE EXCEPTION 'Codex OS profile device pool is incompatible with an existing Spark shadow'
            USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_accounts_codex_identity_shadow_exclusion ON accounts;
CREATE TRIGGER trg_accounts_codex_identity_shadow_exclusion
BEFORE INSERT OR UPDATE OF codex_identity_policy, parent_account_id, quota_dimension, deleted_at ON accounts
FOR EACH ROW EXECUTE FUNCTION enforce_codex_identity_shadow_exclusion();

ALTER TABLE accounts DROP CONSTRAINT IF EXISTS accounts_codex_identity_policy_object_check;
ALTER TABLE accounts
    ADD CONSTRAINT accounts_codex_identity_policy_object_check
    CHECK (jsonb_typeof(codex_identity_policy) = 'object');

ALTER TABLE accounts DROP CONSTRAINT IF EXISTS accounts_codex_identity_mode_exclusive_check;
ALTER TABLE accounts
    ADD CONSTRAINT accounts_codex_identity_mode_exclusive_check
    CHECK (
        COALESCE(codex_identity_policy->>'mode', 'off') <> 'os_profile_device_pool'
        OR COALESCE(NULLIF(extra->>'codex_fingerprint_mode', ''), 'off') = 'off'
    );

ALTER TABLE accounts DROP CONSTRAINT IF EXISTS accounts_codex_identity_active_credentials_check;
ALTER TABLE accounts
    ADD CONSTRAINT accounts_codex_identity_active_credentials_check
    CHECK (
        provisioning_state <> 'active'
        OR COALESCE(codex_identity_policy->>'mode', 'off') <> 'os_profile_device_pool'
        OR (
            platform = 'openai'
            AND type = 'oauth'
            AND (
                NULLIF(BTRIM(credentials->>'access_token'), '') IS NOT NULL
                OR NULLIF(BTRIM(credentials->>'refresh_token'), '') IS NOT NULL
                OR (
                    LOWER(COALESCE(credentials->>'auth_mode', '')) = LOWER('agentIdentity')
                    AND NULLIF(BTRIM(credentials->>'agent_runtime_id'), '') IS NOT NULL
                    AND NULLIF(BTRIM(credentials->>'agent_private_key'), '') IS NOT NULL
                )
            )
        )
    );

ALTER TABLE accounts DROP CONSTRAINT IF EXISTS accounts_codex_identity_active_seed_check;
ALTER TABLE accounts
    ADD CONSTRAINT accounts_codex_identity_active_seed_check
    CHECK (
        provisioning_state <> 'active'
        OR COALESCE(codex_identity_policy->>'mode', 'off') <> 'os_profile_device_pool'
        OR (
            COALESCE(extra->>'codex_fingerprint_seed', '') ~
                '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$'
            AND extra->>'codex_fingerprint_seed' <> '00000000-0000-0000-0000-000000000000'
        )
    );

CREATE INDEX IF NOT EXISTS idx_accounts_provisioning_state
    ON accounts(provisioning_state)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_accounts_schedulable_provisioned_hot
    ON accounts(platform, priority, id)
    WHERE deleted_at IS NULL
      AND status = 'active'
      AND schedulable = TRUE
      AND provisioning_state = 'active';

CREATE TABLE IF NOT EXISTS account_codex_identity_policies (
	id                  BIGSERIAL PRIMARY KEY,
	account_id          BIGINT NOT NULL UNIQUE REFERENCES accounts(id) ON DELETE CASCADE,
    mode                VARCHAR(40) NOT NULL DEFAULT 'off',
    binding_scope       VARCHAR(40) NOT NULL DEFAULT 'api_key_os',
    session_policy      JSONB NOT NULL DEFAULT '{"mode":"conversation_isolated"}'::jsonb,
    affinity_ttl_seconds INTEGER NOT NULL DEFAULT 3600,
    unsupported_policy  VARCHAR(40) NOT NULL DEFAULT 'reject',
    version             BIGINT NOT NULL DEFAULT 1,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT account_codex_identity_policy_mode_check
        CHECK (mode IN ('off', 'os_profile_device_pool')),
    CONSTRAINT account_codex_identity_binding_scope_check
        CHECK (binding_scope = 'api_key_os'),
    CONSTRAINT account_codex_identity_session_policy_check
        CHECK (
            jsonb_typeof(session_policy) = 'object'
            AND COALESCE(session_policy->>'mode', '') IN
                ('conversation_isolated', 'api_key_shared', 'session_pool', 'device_shared')
            AND (
                COALESCE(session_policy->>'mode', '') <> 'device_shared'
                OR (
                    COALESCE((session_policy->>'max_active_conversations_per_slot')::INTEGER, 0) = 1
                    AND COALESCE((session_policy->>'disable_cross_key_continuation')::BOOLEAN, FALSE) = TRUE
                )
            )
        ),
    CONSTRAINT account_codex_identity_affinity_ttl_check
        CHECK (affinity_ttl_seconds BETWEEN 60 AND 86400),
    CONSTRAINT account_codex_identity_unsupported_policy_check
        CHECK (unsupported_policy = 'reject'),
    CONSTRAINT account_codex_identity_version_check
        CHECK (version > 0)
);

CREATE TABLE IF NOT EXISTS account_codex_profiles (
    id                    BIGSERIAL PRIMARY KEY,
    account_id            BIGINT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    os_class              VARCHAR(20) NOT NULL,
    canonical_surface     VARCHAR(20) NOT NULL,
    architecture          VARCHAR(20),
    proxy_id              BIGINT REFERENCES proxies(id) ON DELETE RESTRICT,
    slot_count            INTEGER NOT NULL,
    epoch                 BIGINT NOT NULL,
    catalog_version       BIGINT NOT NULL DEFAULT 1,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT account_codex_profile_os_check
        CHECK (os_class IN ('windows', 'macos', 'linux', 'generic')),
    CONSTRAINT account_codex_profile_surface_check
        CHECK (canonical_surface IN ('desktop', 'cli', 'sdk', 'third_party')),
    CONSTRAINT account_codex_profile_arch_check
        CHECK (architecture IS NULL OR architecture IN ('x86_64', 'arm64')),
    CONSTRAINT account_codex_profile_slot_count_check
        CHECK (slot_count BETWEEN 1 AND 3),
    CONSTRAINT account_codex_profile_epoch_check
        CHECK (epoch > 0),
    CONSTRAINT account_codex_profile_catalog_version_check
        CHECK (catalog_version = 1),
    CONSTRAINT account_codex_profile_shape_check
        CHECK (
            (os_class = 'generic' AND canonical_surface IN ('sdk', 'third_party') AND architecture IS NULL)
            OR
            (os_class <> 'generic' AND canonical_surface IN ('desktop', 'cli') AND architecture IS NOT NULL)
        ),
    UNIQUE(account_id, os_class, epoch),
    UNIQUE(id, account_id)
);

CREATE TABLE IF NOT EXISTS account_codex_device_slots (
    id                    BIGSERIAL PRIMARY KEY,
    account_id            BIGINT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    profile_id            BIGINT NOT NULL,
    slot_index            INTEGER NOT NULL,
    proxy_id              BIGINT REFERENCES proxies(id) ON DELETE RESTRICT,
    epoch                 BIGINT NOT NULL,
    state                 VARCHAR(20) NOT NULL DEFAULT 'active',
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT account_codex_slot_profile_fk
        FOREIGN KEY(profile_id, account_id)
        REFERENCES account_codex_profiles(id, account_id)
        ON DELETE CASCADE,
    CONSTRAINT account_codex_slot_index_check CHECK (slot_index BETWEEN 0 AND 2),
    CONSTRAINT account_codex_slot_epoch_check CHECK (epoch > 0),
    CONSTRAINT account_codex_slot_state_check CHECK (state IN ('active', 'draining')),
    UNIQUE(profile_id, slot_index, epoch),
    UNIQUE(id, account_id)
);

CREATE TABLE IF NOT EXISTS account_codex_device_bindings (
    id                    BIGSERIAL PRIMARY KEY,
    account_id            BIGINT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    api_key_id            BIGINT NOT NULL REFERENCES api_keys(id) ON DELETE CASCADE,
    os_class              VARCHAR(20) NOT NULL,
    slot_id               BIGINT NOT NULL,
    policy_version        BIGINT NOT NULL,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT account_codex_binding_slot_fk
        FOREIGN KEY(slot_id, account_id)
        REFERENCES account_codex_device_slots(id, account_id)
        ON DELETE CASCADE,
    CONSTRAINT account_codex_binding_os_check
        CHECK (os_class IN ('windows', 'macos', 'linux', 'generic')),
    CONSTRAINT account_codex_binding_version_check CHECK (policy_version > 0),
    UNIQUE(account_id, api_key_id, os_class)
);

INSERT INTO account_codex_identity_policies
    (account_id, mode, binding_scope, session_policy, affinity_ttl_seconds, unsupported_policy, version)
SELECT id,
       'off',
       'api_key_os',
       '{"mode":"conversation_isolated"}'::jsonb,
       3600,
       'reject',
       1
FROM accounts
WHERE deleted_at IS NULL
ON CONFLICT (account_id) DO NOTHING;

CREATE INDEX IF NOT EXISTS idx_account_codex_profiles_account
    ON account_codex_profiles(account_id, os_class, epoch);
CREATE INDEX IF NOT EXISTS idx_account_codex_slots_account_state
    ON account_codex_device_slots(account_id, state);
CREATE INDEX IF NOT EXISTS idx_account_codex_bindings_api_key
    ON account_codex_device_bindings(api_key_id, os_class);
