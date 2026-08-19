-- 用户画像分群：独立于模型分组（groups）
-- user_tags 是管理员维护的用户标签字典；user_tag_assignments 是用户与标签的多对多关系。
CREATE TABLE IF NOT EXISTS user_tags (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(80) NOT NULL,
    normalized_name VARCHAR(80) NOT NULL,
    color VARCHAR(20) NOT NULL DEFAULT '#6366f1',
    description TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_user_tags_normalized_name_active
    ON user_tags (normalized_name)
    WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_user_tags_active_sort
    ON user_tags (deleted_at, name, id);

CREATE TABLE IF NOT EXISTS user_tag_assignments (
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    tag_id BIGINT NOT NULL REFERENCES user_tags(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, tag_id)
);
CREATE INDEX IF NOT EXISTS idx_user_tag_assignments_tag_user
    ON user_tag_assignments (tag_id, user_id);

-- 用户隐藏的模型分组。该表只描述用户侧“不可见/不可选”的模型分组，
-- 不改变 groups 本身的模型配置，也不取代 user_allowed_groups 的授权语义。
CREATE TABLE IF NOT EXISTS user_hidden_groups (
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    group_id BIGINT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, group_id)
);
CREATE INDEX IF NOT EXISTS idx_user_hidden_groups_group_user
    ON user_hidden_groups (group_id, user_id);
