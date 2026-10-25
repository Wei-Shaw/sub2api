-- MySQL 方言版本（翻译自 ../075_map_haiku45_to_sonnet46.sql）
-- Map claude-haiku-4-5 variants target from claude-sonnet-4-5 to claude-sonnet-4-6
--
-- Only updates when the current target is exactly claude-sonnet-4-5.
--
-- 翻译说明：jsonb_set(credentials, '{model_mapping,<key>}', '"claude-sonnet-4-6"')
--           → JSON_SET(credentials, '$.model_mapping."<key>"', 'claude-sonnet-4-6')
--             （MySQL JSON_SET 的字符串实参按 JSON 字符串存储，等价于 PG 的 '"..."'::jsonb）；
--           含连字符的键名需用双引号括起；->> 取值 → JSON_UNQUOTE(JSON_EXTRACT(...))。

-- 1. claude-haiku-4-5
UPDATE accounts
SET credentials = JSON_SET(credentials, '$.model_mapping."claude-haiku-4-5"', 'claude-sonnet-4-6')
WHERE platform = 'antigravity'
  AND deleted_at IS NULL
  AND JSON_UNQUOTE(JSON_EXTRACT(credentials, '$.model_mapping."claude-haiku-4-5"')) = 'claude-sonnet-4-5';

-- 2. claude-haiku-4-5-20251001
UPDATE accounts
SET credentials = JSON_SET(credentials, '$.model_mapping."claude-haiku-4-5-20251001"', 'claude-sonnet-4-6')
WHERE platform = 'antigravity'
  AND deleted_at IS NULL
  AND JSON_UNQUOTE(JSON_EXTRACT(credentials, '$.model_mapping."claude-haiku-4-5-20251001"')) = 'claude-sonnet-4-5';
