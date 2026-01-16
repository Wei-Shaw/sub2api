-- 040_add_group_model_routing.sql
-- 添加分组级别的模型路由配置功能

-- 添加 model_routing 字段：模型路由配置（JSONB 格式）
-- 格式: {"model_pattern": [account_id1, account_id2], ...REDACTED
-- 例如: {"claude-opus-*": [1, 2], "claude-sonnet-*": [3, 4, 5]REDACTED
ALTER TABLE groups
ADD COLUMN IF NOT EXISTS model_routing JSONB DEFAULT '{REDACTED';

-- 添加字段注释
COMMENT ON COLUMN groups.model_routing IS '模型路由配置：{"model_pattern": [account_id1, account_id2], ...REDACTED，支持通配符匹配';
