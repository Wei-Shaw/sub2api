-- MySQL 方言版本（翻译自 ../029_add_group_claude_code_restriction.sql）
-- 029_add_group_claude_code_restriction.sql
-- 添加分组级别的 Claude Code 客户端限制功能
--
-- 翻译说明：
--   - ADD COLUMN IF NOT EXISTS → 去掉修饰词；`groups` 为 MySQL 8 保留字；
--   - PG 内联 REFERENCES → 先加列，再显式命名 FK 约束（InnoDB 复用下方显式索引）；
--   - 非唯一部分索引（WHERE 谓词）→ 去掉 WHERE，改普通索引（仅性能差异）；
--   - COMMENT ON COLUMN → 整句跳过。

-- 添加 claude_code_only 字段：是否仅允许 Claude Code 客户端
ALTER TABLE `groups`
ADD COLUMN claude_code_only BOOLEAN NOT NULL DEFAULT FALSE;

-- 添加 fallback_group_id 字段：非 Claude Code 请求降级到的分组
ALTER TABLE `groups`
ADD COLUMN fallback_group_id BIGINT NULL;

-- 添加索引优化查询
-- PG 部分索引（WHERE deleted_at IS NULL）→ 普通索引
CREATE INDEX idx_groups_claude_code_only
ON `groups`(claude_code_only);

-- PG 部分索引（WHERE deleted_at IS NULL AND fallback_group_id IS NOT NULL）→ 普通索引
CREATE INDEX idx_groups_fallback_group_id
ON `groups`(fallback_group_id);

ALTER TABLE `groups`
ADD CONSTRAINT fk_groups_fallback_group_id
FOREIGN KEY (fallback_group_id) REFERENCES `groups`(id) ON DELETE SET NULL;

-- 添加字段注释
-- (comment skipped) COMMENT ON COLUMN groups.claude_code_only IS '是否仅允许 Claude Code 客户端访问此分组'
-- (comment skipped) COMMENT ON COLUMN groups.fallback_group_id IS '非 Claude Code 请求降级使用的分组 ID'
