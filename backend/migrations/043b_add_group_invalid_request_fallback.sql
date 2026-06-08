-- 043_add_group_invalid_request_fallback.sql
-- 添加无效请求兜底分组配置

-- 添加 fallback_group_id_on_invalid_request 字段：无效请求兜底使用的分组
ALTER TABLE groups
ADD COLUMN IF NOT EXISTS fallback_group_id_on_invalid_request BIGINT REFERENCES groups(id) ON DELETE SET NULL;

-- 添加索引优化查询
CREATE INDEX IF NOT EXISTS idx_groups_fallback_group_id_on_invalid_request
ON groups(fallback_group_id_on_invalid_request) WHERE deleted_at IS NULL AND fallback_group_id_on_invalid_request IS NOT NULL;

-- 添加字段注释
COMMENT ON COLUMN groups.fallback_group_id_on_invalid_request IS '无效请求兜底使用的分组 ID';
