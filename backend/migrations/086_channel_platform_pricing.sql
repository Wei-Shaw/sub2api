-- 086_channel_platform_pricing.sql
-- 渠道按平台维度：model_pricing 加 platform 列，model_mapping 改为嵌套格式

-- 1. channel_model_pricing 加 platform 列
ALTER TABLE channel_model_pricing
    ADD COLUMN IF NOT EXISTS platform VARCHAR(50) NOT NULL DEFAULT 'anthropic';

CREATE INDEX IF NOT EXISTS idx_channel_model_pricing_platform
    ON channel_model_pricing (platform);

-- 2. model_mapping: 从扁平 {"src":"dst"} 迁移为嵌套 {"anthropic":{"src":"dst"}}
-- 仅迁移非空、非 '{}' 的旧格式数据（通过检查第一个 value 是否为字符串来判断是否为旧格式）
UPDATE channels
SET model_mapping = jsonb_build_object('anthropic', model_mapping)
WHERE model_mapping IS NOT NULL
  AND model_mapping::text NOT IN ('{}', 'null', '')
  AND NOT EXISTS (
      SELECT 1 FROM jsonb_each(model_mapping) AS kv
      WHERE jsonb_typeof(kv.value) = 'object'
      LIMIT 1
  );
