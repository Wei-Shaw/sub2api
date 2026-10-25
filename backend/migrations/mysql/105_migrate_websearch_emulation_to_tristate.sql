-- MySQL 方言版本（翻译自 ../105_migrate_websearch_emulation_to_tristate.sql）
-- 翻译说明：
--   - extra ? 'web_search_emulation' → JSON_CONTAINS_PATH(extra, 'one', '$.web_search_emulation')。
--   - extra->>'web_search_emulation' → JSON_UNQUOTE(JSON_EXTRACT(extra, '$.web_search_emulation'))。
--   - (extra - 'k') || jsonb_build_object('k', 'enabled')（删键后合并覆盖）
--     → JSON_SET(extra, '$.k', 'enabled')（对已存在键覆盖写，净效果等价；
--       SQL 字符串实参写入即 JSON 字符串 "enabled"）。
--   - extra - 'k' → JSON_REMOVE(extra, '$.k')。

-- Convert old boolean web_search_emulation to tri-state string
-- true → "enabled", false → remove key (becomes "default")
UPDATE accounts
SET extra = JSON_SET(extra, '$.web_search_emulation', 'enabled')
WHERE JSON_CONTAINS_PATH(extra, 'one', '$.web_search_emulation')
  AND JSON_UNQUOTE(JSON_EXTRACT(extra, '$.web_search_emulation')) = 'true';

UPDATE accounts
SET extra = JSON_REMOVE(extra, '$.web_search_emulation')
WHERE JSON_CONTAINS_PATH(extra, 'one', '$.web_search_emulation')
  AND JSON_UNQUOTE(JSON_EXTRACT(extra, '$.web_search_emulation')) = 'false';
