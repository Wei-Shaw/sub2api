-- 修复 024_add_gemini_tier_id.sql 的自我回滚。
--
-- 024 曾包含 goose Down 块，但迁移运行器不解析 goose 指令，Down 块会紧接着
-- Up 块在同一事务里执行，`UPDATE accounts SET credentials = credentials - 'tier_id'`
-- 立刻抹掉了刚写入的默认值，导致 Gemini Code Assist OAuth 账号始终拿不到 LEGACY 默认 tier。
--
-- 024 已清理 goose 标记；本迁移在已记录过 024 的旧库上重新执行 024 的 Up 语句。
-- WHERE 条件保证幂等：已有 tier_id 的账号不会被覆盖。

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
