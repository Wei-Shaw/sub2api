-- MySQL 方言版本（翻译自 ../086_channel_platform_pricing.sql）
-- 086_channel_platform_pricing.sql
-- 渠道按平台维度：model_pricing 加 platform 列，model_mapping 改为嵌套格式
--
-- 翻译说明：
-- - jsonb_build_object('anthropic', model_mapping) → JSON_OBJECT('anthropic', model_mapping)
--   （model_mapping 为 JSON 列，JSON_OBJECT 按原生 JSON 值嵌入，而非字符串）；
-- - model_mapping::text → CAST(model_mapping AS CHAR)（MySQL 的 JSON → CHAR 得到 JSON 文本，
--   与 PG 的 ::text 一样可比较 '{}', 'null', ''）；
-- - 旧格式判断（jsonb_each + jsonb_typeof 子查询：不存在 object 类型的顶层成员）
--   → JSON_EXTRACT(model_mapping, '$.*') 得到顶层成员值数组，
--     JSON_TABLE '$[*]' 展开 + JSON_TYPE(v) = 'OBJECT' 判断
--     （MySQL 的 JSON_TYPE 返回大写 'OBJECT'，PG 的 jsonb_typeof 为小写 'object'）；
-- - PG 的 SET LOCAL lock_timeout / statement_timeout 无 MySQL 等价语句，跳过。

-- 1. channel_model_pricing 加 platform 列
ALTER TABLE channel_model_pricing
    ADD COLUMN platform VARCHAR(50) NOT NULL DEFAULT 'anthropic';

CREATE INDEX idx_channel_model_pricing_platform
    ON channel_model_pricing (platform);

-- 2. model_mapping: 从扁平 {"src":"dst"} 迁移为嵌套 {"anthropic":{"src":"dst"}}
-- 仅迁移非空、非 '{}' 的旧格式数据（通过检查第一个 value 是否为字符串来判断是否为旧格式）
UPDATE channels
SET model_mapping = JSON_OBJECT('anthropic', model_mapping)
WHERE model_mapping IS NOT NULL
  AND CAST(model_mapping AS CHAR) NOT IN ('{}', 'null', '')
  AND NOT EXISTS (
      SELECT 1
      FROM JSON_TABLE(
              JSON_EXTRACT(model_mapping, '$.*'),
              '$[*]' COLUMNS (v JSON PATH '$')
           ) AS kv
      WHERE JSON_TYPE(kv.v) = 'OBJECT'
  );
