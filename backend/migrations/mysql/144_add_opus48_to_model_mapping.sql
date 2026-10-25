-- MySQL 方言版本（翻译自 ../144_add_opus48_to_model_mapping.sql）
-- 翻译说明：
--   - jsonb_set(credentials, '{model_mapping,claude-opus-4-8}', '"claude-opus-4-8"'::jsonb, true)
--     → JSON_SET(credentials, '$.model_mapping."claude-opus-4-8"', CAST(... AS JSON))
--     （create_missing 语义一致；键名含连字符，JSON 路径内用双引号）。
--   - jsonb_typeof(x) = 'object' → JSON_TYPE(x) = 'OBJECT'；
--     x->'a'->>'b' → JSON_UNQUOTE(JSON_EXTRACT(x, '$.a."b"'))。

-- 为已持久化的 Antigravity model_mapping 添加 claude-opus-4-8。
--
-- 未持久化 model_mapping 的账号会直接使用 DefaultAntigravityModelMapping，
-- 因此这里只需要回填已有映射对象。

UPDATE accounts
SET credentials = JSON_SET(
    credentials,
    '$.model_mapping."claude-opus-4-8"',
    CAST('claude-opus-4-8' AS JSON)
)
WHERE platform = 'antigravity'
  AND deleted_at IS NULL
  AND JSON_TYPE(JSON_EXTRACT(credentials, '$.model_mapping')) = 'OBJECT'
  AND JSON_UNQUOTE(JSON_EXTRACT(credentials, '$.model_mapping."claude-opus-4-8"')) IS NULL;
