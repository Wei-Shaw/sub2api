-- MySQL 方言版本（翻译自 ../050_map_opus46_to_opus45.sql）
-- Map claude-opus-4-6 to claude-opus-4-5-thinking
--
-- Notes:
-- - Updates existing Antigravity accounts' model_mapping
-- - Changes claude-opus-4-6 target from claude-opus-4-6 to claude-opus-4-5-thinking
-- - This is needed because previous versions didn't have this mapping
--
-- 翻译说明：jsonb_set(credentials, '{model_mapping,claude-opus-4-6}', ...)

UPDATE accounts
SET credentials = JSON_SET(
    credentials,
    '$.model_mapping."claude-opus-4-6"',
    CAST('"claude-opus-4-5-thinking"' AS JSON)
)
WHERE platform = 'antigravity'
  AND deleted_at IS NULL
  AND JSON_EXTRACT(credentials, '$.model_mapping') IS NOT NULL
  AND JSON_UNQUOTE(JSON_EXTRACT(credentials, '$.model_mapping."claude-opus-4-6"')) IS NOT NULL;
