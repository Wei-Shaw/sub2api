CREATE TABLE IF NOT EXISTS user_pool_group_grants (
    pool_id         BIGINT NOT NULL REFERENCES user_pools(id) ON DELETE CASCADE,
    group_id        BIGINT NOT NULL REFERENCES groups(id)     ON DELETE CASCADE,
    rate_multiplier DECIMAL(10,4),
    rpm_override    INTEGER,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (pool_id, group_id),
    CONSTRAINT chk_user_pool_group_grants_rate CHECK (rate_multiplier IS NULL OR rate_multiplier > 0),
    CONSTRAINT chk_user_pool_group_grants_rpm CHECK (rpm_override IS NULL OR rpm_override >= 0)
);

CREATE INDEX IF NOT EXISTS idx_user_pool_group_grants_group_id
    ON user_pool_group_grants (group_id);
