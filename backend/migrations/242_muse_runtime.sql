-- Native Muse runtime ownership. No account type, routes, or transport enabled.
-- A remote workspace has one downstream owner even when imported more than once.
CREATE TABLE IF NOT EXISTS muse_workspace_bindings (
    id BIGSERIAL PRIMARY KEY,
    principal_id VARCHAR(256) NOT NULL,
    remote_workspace_id VARCHAR(256) NOT NULL,
    owner_user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    identity_generation BIGINT NOT NULL DEFAULT 1 CHECK (identity_generation > 0),
    lease_fence BIGINT NOT NULL DEFAULT 0 CHECK (lease_fence >= 0),
    active_turn_id VARCHAR(96),
    lease_owner VARCHAR(96),
    lease_until TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (principal_id, remote_workspace_id),
    CHECK ((active_turn_id IS NULL AND lease_owner IS NULL AND lease_until IS NULL)
        OR (active_turn_id IS NOT NULL AND lease_owner IS NOT NULL AND lease_until IS NOT NULL))
);

-- Alias membership is validated against the account snapshot observed by auth.
CREATE TABLE IF NOT EXISTS muse_account_workspace_bindings (
    account_id BIGINT PRIMARY KEY REFERENCES accounts(id) ON DELETE RESTRICT,
    workspace_id BIGINT NOT NULL REFERENCES muse_workspace_bindings(id) ON DELETE RESTRICT,
    verified_account_updated_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS muse_turns (
    id VARCHAR(96) PRIMARY KEY,
    workspace_id BIGINT NOT NULL REFERENCES muse_workspace_bindings(id) ON DELETE RESTRICT,
    identity_generation BIGINT NOT NULL CHECK (identity_generation > 0),
    owner_user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    api_key_id BIGINT NOT NULL REFERENCES api_keys(id) ON DELETE RESTRICT,
    account_id BIGINT NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
    state VARCHAR(32) NOT NULL DEFAULT 'reserved',
    provider_turn_id VARCHAR(256) NOT NULL DEFAULT '',
    pricing_snapshot JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT muse_turn_state_check CHECK (state IN
        ('reserved', 'submitting', 'accepted', 'running', 'cancel_pending',
         'ambiguous', 'owner_review', 'completed', 'failed', 'cancelled', 'rejected'))
);

CREATE UNIQUE INDEX IF NOT EXISTS muse_turns_one_active_workspace
    ON muse_turns(workspace_id)
    WHERE state NOT IN ('completed', 'failed', 'cancelled', 'rejected');
CREATE INDEX IF NOT EXISTS muse_turns_owner_key ON muse_turns(owner_user_id, api_key_id);
CREATE INDEX IF NOT EXISTS muse_workspace_expired_lease
    ON muse_workspace_bindings(lease_until) WHERE active_turn_id IS NOT NULL;
