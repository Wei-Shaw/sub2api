CREATE TABLE IF NOT EXISTS user_pools (
    id          BIGSERIAL PRIMARY KEY,
    name        VARCHAR(100) NOT NULL,
    description TEXT,
    status      VARCHAR(20) NOT NULL DEFAULT 'active',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at  TIMESTAMPTZ,
    CONSTRAINT chk_user_pools_status CHECK (status IN ('active', 'disabled'))
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_user_pools_name_active
    ON user_pools (name) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_user_pools_status ON user_pools (status);
CREATE INDEX IF NOT EXISTS idx_user_pools_deleted_at ON user_pools (deleted_at);
