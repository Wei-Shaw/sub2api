CREATE TABLE IF NOT EXISTS user_pool_members (
    pool_id    BIGINT NOT NULL REFERENCES user_pools(id) ON DELETE CASCADE,
    user_id    BIGINT NOT NULL REFERENCES users(id)      ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (pool_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_user_pool_members_user_id
    ON user_pool_members (user_id);
