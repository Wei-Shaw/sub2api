-- MySQL 方言版本（翻译自 ../007_add_user_allowed_groups.sql）
-- Add user_allowed_groups join table to replace users.allowed_groups (BIGINT[]).
-- Phase 1: create table + backfill from the legacy array column.
--
-- 翻译说明：
--   - BIGINT[] → JSON；CROSS JOIN LATERAL unnest(u.allowed_groups) → JSON_TABLE 展开
--     （legacy allowed_groups 元素约定为 JSON 数字，与 PG BIGINT[] 语义对齐）；
--   - ON CONFLICT DO NOTHING（无 id 列）→ ON DUPLICATE KEY UPDATE 首列自赋值；
--   - `groups` 为 MySQL 8 保留字。

CREATE TABLE IF NOT EXISTS user_allowed_groups (
    user_id     BIGINT NOT NULL,
    group_id    BIGINT NOT NULL,
    created_at  DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (user_id, group_id),
    CONSTRAINT fk_user_allowed_groups_user_id FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    CONSTRAINT fk_user_allowed_groups_group_id FOREIGN KEY (group_id) REFERENCES `groups`(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE INDEX idx_user_allowed_groups_group_id ON user_allowed_groups(group_id);

-- Backfill from the legacy users.allowed_groups array.
-- TODO-REVIEW: PG 版 BIGINT[] 元素必为整数；MySQL 侧 JSON 元素若为非数字（如对象/非法字符串），
-- JSON_TABLE 转换结果为 NULL，JOIN `groups` 不命中而被静默跳过（PG 侧此类输入不可能出现）。
-- 数字字符串（如 "5"）可正常转换，正常数据（JSON 数字数组）行为与 PG 完全一致。
INSERT INTO user_allowed_groups (user_id, group_id)
SELECT u.id, jt.group_id
FROM users u
JOIN JSON_TABLE(u.allowed_groups, '$[*]'
        COLUMNS (group_id BIGINT PATH '$')) jt
JOIN `groups` g ON g.id = jt.group_id
WHERE u.allowed_groups IS NOT NULL
ON DUPLICATE KEY UPDATE user_id = user_id;
