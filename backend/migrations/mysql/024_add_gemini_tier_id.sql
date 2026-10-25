-- MySQL 方言版本（翻译自 ../024_add_gemini_tier_id.sql）
--
-- 翻译说明：
--   - goose 指令注释（+goose Up/Down/StatementBegin/StatementEnd）非 SQL 语义，翻译时去掉；
--     执行器整文件执行，Down 段不能保留，故不翻译；
--   - jsonb_set(credentials, '{tier_id}', '"LEGACY"', true) → JSON_SET(credentials, '$.tier_id', 'LEGACY')
--     （JSON_SET 默认创建缺失键，等价于 create_missing=true；'LEGACY' 存为 JSON 字符串，同 PG 的 "LEGACY"）；
--   - jsonb_typeof(...) = 'object' → JSON_TYPE(...) = 'OBJECT'；
--   - credentials->>'k' → JSON_UNQUOTE(JSON_EXTRACT(credentials, '$.k'))。

-- 为 Gemini Code Assist OAuth 账号添加默认 tier_id
-- 包括显式标记为 code_assist 的账号，以及 legacy 账号（oauth_type 为空但 project_id 存在）
UPDATE accounts
SET credentials = JSON_SET(credentials, '$.tier_id', 'LEGACY')
WHERE platform = 'gemini'
  AND type = 'oauth'
  AND JSON_TYPE(credentials) = 'OBJECT'
  AND JSON_UNQUOTE(JSON_EXTRACT(credentials, '$.tier_id')) IS NULL
  AND (
    JSON_UNQUOTE(JSON_EXTRACT(credentials, '$.oauth_type')) = 'code_assist'
    OR (JSON_UNQUOTE(JSON_EXTRACT(credentials, '$.oauth_type')) IS NULL
        AND JSON_UNQUOTE(JSON_EXTRACT(credentials, '$.project_id')) IS NOT NULL)
  );

-- （PG 版 Down 段不翻译：credentials - 'tier_id' → 对应 JSON_REMOVE(credentials, '$.tier_id')，
--   条件 credentials ? 'tier_id' → JSON_CONTAINS_PATH(credentials, 'one', '$.tier_id')。）
