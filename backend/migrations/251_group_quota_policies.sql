-- Group-scoped, opt-in quota model rules. Empty configuration never changes routing.
CREATE TABLE IF NOT EXISTS group_quota_policies (
    group_id BIGINT PRIMARY KEY REFERENCES groups(id) ON DELETE CASCADE,
    revision BIGINT NOT NULL DEFAULT 1 CHECK (revision > 0),
    runtime_version BIGINT NOT NULL DEFAULT 1 CHECK (runtime_version > 0),
    config JSONB NOT NULL DEFAULT '{"rules":[]}'::jsonb,
    runtime JSONB NOT NULL DEFAULT '{"snapshots":[],"rules":[],"events":[]}'::jsonb,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
