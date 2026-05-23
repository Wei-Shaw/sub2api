CREATE TABLE billing_pools (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    code VARCHAR(100) NOT NULL,
    description TEXT,
    status VARCHAR(20) NOT NULL DEFAULT 'active',
    platform_scope VARCHAR(32) NOT NULL DEFAULT 'same_platform',
    allow_user_reorder BOOLEAN NOT NULL DEFAULT FALSE,
    require_primary_subscription BOOLEAN NOT NULL DEFAULT TRUE,
    allow_balance_fallback BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ NULL
);

CREATE UNIQUE INDEX billing_pools_code_unique_active
    ON billing_pools(code)
    WHERE deleted_at IS NULL;

CREATE INDEX billing_pools_status_idx ON billing_pools(status);
CREATE INDEX billing_pools_deleted_at_idx ON billing_pools(deleted_at);

CREATE TABLE billing_pool_groups (
    id BIGSERIAL PRIMARY KEY,
    billing_pool_id BIGINT NOT NULL REFERENCES billing_pools(id),
    group_id BIGINT NOT NULL REFERENCES groups(id),
    chain_order INT NOT NULL DEFAULT 0,
    can_be_primary BOOLEAN NOT NULL DEFAULT TRUE,
    can_be_fallback BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ NULL
);

CREATE UNIQUE INDEX billing_pool_groups_unique_active
    ON billing_pool_groups(billing_pool_id, group_id)
    WHERE deleted_at IS NULL;

CREATE INDEX billing_pool_groups_pool_order_idx
    ON billing_pool_groups(billing_pool_id, chain_order)
    WHERE deleted_at IS NULL;

CREATE UNIQUE INDEX billing_pool_groups_group_single_pool_active
    ON billing_pool_groups(group_id)
    WHERE deleted_at IS NULL;

CREATE INDEX billing_pool_groups_deleted_at_idx ON billing_pool_groups(deleted_at);

ALTER TABLE api_keys
    ADD COLUMN billing_mode VARCHAR(64) NOT NULL DEFAULT 'primary_then_balance',
    ADD COLUMN billing_pool_id BIGINT NULL REFERENCES billing_pools(id),
    ADD COLUMN custom_fallback_group_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN use_pool_default_order BOOLEAN NOT NULL DEFAULT TRUE;

CREATE INDEX api_keys_billing_pool_id_idx ON api_keys(billing_pool_id);
CREATE INDEX api_keys_billing_mode_idx ON api_keys(billing_mode);
