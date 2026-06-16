-- 138_add_group_model_mapping.sql
-- 为分组添加模型映射配置：支持通配符（末尾 *）和正则（~ 前缀）匹配，可在分组级别对请求模型进行透明重写。
ALTER TABLE groups ADD COLUMN IF NOT EXISTS model_mapping JSONB NOT NULL DEFAULT '{}';
