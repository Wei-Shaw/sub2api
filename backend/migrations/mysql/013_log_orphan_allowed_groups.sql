-- MySQL 方言版本（翻译自 ../013_log_orphan_allowed_groups.sql）
-- 013: 记录 users.allowed_groups 中的孤立 group_id
-- 任务：fix-medium-data-hygiene 3.1
--
-- 目的：在删除 legacy allowed_groups 列前，记录所有引用了不存在 group 的孤立记录
-- 这些记录可用于审计或后续数据修复
--
-- 翻译说明：
--   - BIGSERIAL → BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY；TIMESTAMPTZ → DATETIME(3)；
--   - CROSS JOIN LATERAL unnest(u.allowed_groups) → JSON_TABLE 展开
--     （allowed_groups 元素约定为 JSON 数字，与 PG BIGINT[] 语义对齐）；
--   - ON CONFLICT (user_id, group_id) DO NOTHING → ON DUPLICATE KEY UPDATE id = id；
--   - COMMENT ON TABLE → 整句跳过；`groups` 为 MySQL 8 保留字。

-- 创建审计表存储孤立的 allowed_groups 记录
CREATE TABLE IF NOT EXISTS orphan_allowed_groups_audit (
    id          BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    user_id     BIGINT NOT NULL,
    group_id    BIGINT NOT NULL,
    recorded_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    UNIQUE (user_id, group_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- 记录孤立的 group_id（存在于 users.allowed_groups 但不存在于 groups 表）
INSERT INTO orphan_allowed_groups_audit (user_id, group_id)
SELECT u.id, jt.group_id
FROM users u
JOIN JSON_TABLE(u.allowed_groups, '$[*]'
        COLUMNS (group_id BIGINT PATH '$')) jt
LEFT JOIN `groups` g ON g.id = jt.group_id
WHERE u.allowed_groups IS NOT NULL
  AND g.id IS NULL
ON DUPLICATE KEY UPDATE id = orphan_allowed_groups_audit.id;

-- 添加索引便于查询
CREATE INDEX idx_orphan_allowed_groups_audit_user_id
ON orphan_allowed_groups_audit(user_id);

-- 记录迁移完成信息
-- (comment skipped) COMMENT ON TABLE orphan_allowed_groups_audit IS
-- '审计表：记录 users.allowed_groups 中引用的不存在的 group_id，用于数据清理前的审计'
