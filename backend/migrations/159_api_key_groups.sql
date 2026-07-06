CREATE TABLE IF NOT EXISTS api_key_groups (
    api_key_id BIGINT NOT NULL REFERENCES api_keys(id) ON DELETE CASCADE,
    group_id BIGINT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    priority INTEGER NOT NULL DEFAULT 50,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (api_key_id, group_id)
);

CREATE INDEX IF NOT EXISTS idx_api_key_groups_group_id ON api_key_groups (group_id);
CREATE INDEX IF NOT EXISTS idx_api_key_groups_priority ON api_key_groups (priority);
CREATE INDEX IF NOT EXISTS idx_api_key_groups_group_priority_key ON api_key_groups (group_id, priority, api_key_id);

INSERT INTO api_key_groups (api_key_id, group_id)
SELECT id, group_id
FROM api_keys
WHERE group_id IS NOT NULL
ON CONFLICT (api_key_id, group_id) DO NOTHING;
