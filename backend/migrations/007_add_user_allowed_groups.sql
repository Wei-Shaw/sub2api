-- Add user_allowed_groups join table to replace users.allowed_groups (BIGINT[]).
-- Phase 1: create table + backfill from the legacy array column.

CREATE TABLE IF NOT EXISTS user_allowed_groups (
    user_id     BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    group_id    BIGINT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, group_id)
);

CREATE INDEX IF NOT EXISTS idx_user_allowed_groups_group_id ON user_allowed_groups(group_id);

-- Backfill from the legacy users.allowed_groups array.
INSERT INTO user_allowed_groups (user_id, group_id)
SELECT u.id, x.group_id
FROM users u
CROSS JOIN LATERAL unnest(u.allowed_groups) AS x(group_id)
JOIN groups g ON g.id = x.group_id
WHERE u.allowed_groups IS NOT NULL
ON CONFLICT DO NOTHING;
