-- MySQL 方言版本（翻译自 ../043b_add_group_invalid_request_fallback.sql）
-- 043_add_group_invalid_request_fallback.sql
-- 添加无效请求兜底分组配置

-- 添加 fallback_group_id_on_invalid_request 字段：无效请求兜底使用的分组
-- （PG 内联 REFERENCES → 显式命名 FK 约束；`groups` 为 MySQL 8 保留字）
ALTER TABLE `groups`
ADD COLUMN fallback_group_id_on_invalid_request BIGINT NULL;

ALTER TABLE `groups`
ADD CONSTRAINT fk_groups_fallback_group_id_on_invalid_request
FOREIGN KEY (fallback_group_id_on_invalid_request) REFERENCES `groups`(id) ON DELETE SET NULL;

-- 添加索引优化查询
-- PG 部分索引（WHERE deleted_at IS NULL AND fallback_group_id_on_invalid_request IS NOT NULL）
-- → 非唯一部分索引 → 去掉 WHERE（普通索引，仅性能差异）
CREATE INDEX idx_groups_fallback_group_id_on_invalid_request
ON `groups`(fallback_group_id_on_invalid_request);

-- 添加字段注释
-- (comment skipped)
