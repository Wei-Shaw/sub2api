-- +goose Up
-- +goose StatementBegin
-- 为 Gemini Code Assist OAuth 账号添加默认 tier_id
-- 包括显式标记为 code_assist 的账号，以及 legacy 账号（oauth_type 为空但 project_id 存在）
UPDATE accounts
SET credentials = jsonb_set(
    credentials,
    '{tier_id}',
    '"LEGACY"',
    true
)
WHERE platform = 'gemini'
  AND type = 'oauth'
  AND jsonb_typeof(credentials) = 'object'
  AND credentials->>'tier_id' IS NULL
  AND (
    credentials->>'oauth_type' = 'code_assist'
    OR (credentials->>'oauth_type' IS NULL AND credentials->>'project_id' IS NOT NULL)
  );
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- 回滚：删除 tier_id 字段
UPDATE accounts
SET credentials = credentials - 'tier_id'
WHERE platform = 'gemini'
  AND type = 'oauth'
  AND credentials ? 'tier_id';
-- +goose StatementEnd
