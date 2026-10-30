-- Opt-in management pools and persistent, shared-exit admission state.
CREATE TABLE IF NOT EXISTS account_scheduling_pools (
 id BIGSERIAL PRIMARY KEY, name VARCHAR(100) NOT NULL, enabled BOOLEAN NOT NULL DEFAULT FALSE,
 created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(), updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE TABLE IF NOT EXISTS account_scheduling_members (
 id BIGSERIAL PRIMARY KEY, account_id BIGINT NOT NULL UNIQUE REFERENCES accounts(id) ON DELETE CASCADE,
 pool_id BIGINT NOT NULL REFERENCES account_scheduling_pools(id) ON DELETE CASCADE,
 position BIGINT NOT NULL DEFAULT 0,
 created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(), updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS account_scheduling_members_pool_position_idx ON account_scheduling_members(pool_id,position);
CREATE TABLE IF NOT EXISTS account_scheduling_slots (
 id BIGSERIAL PRIMARY KEY, identity VARCHAR(64) NOT NULL UNIQUE, account_id BIGINT NOT NULL,
 exit_group VARCHAR(100) NOT NULL, state VARCHAR(20) NOT NULL DEFAULT 'standby', reason VARCHAR(100) NOT NULL DEFAULT 'ready',
 queued_at TIMESTAMPTZ NOT NULL DEFAULT NOW(), created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(), updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS account_scheduling_slots_exit_state_idx ON account_scheduling_slots(exit_group,state);
CREATE TABLE IF NOT EXISTS account_scheduling_leases (
 id BIGSERIAL PRIMARY KEY, lease_id VARCHAR(100) NOT NULL UNIQUE,
 identity VARCHAR(64) NOT NULL REFERENCES account_scheduling_slots(identity) ON DELETE CASCADE,
 expires_at TIMESTAMPTZ NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(), updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS account_scheduling_leases_identity_expiry_idx ON account_scheduling_leases(identity,expires_at);
