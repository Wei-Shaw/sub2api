-- Migration: 114_fix_claude_code_template_userid
-- 113 的 seed 使用 legacy 格式的 metadata.user_id，但已部署环境此前是手工建的
-- 「Claude Code 伪装」模板（用新版 JSON-string 格式 user_id），113 的 ON CONFLICT
-- DO NOTHING 不会覆盖。本 migration 定向修复这一条历史记录及其下游监控快照。
--
-- 安全性：WHERE 条件同时匹配 (provider, name) + user_id 以 '{' 开头，
-- 所以：
--   - 用户自己改过 user_id（或者 seed 本来就是 legacy）→ LIKE 不中，保持原状
--   - 用户改过 template name / provider → WHERE 不中，完全跳过
-- 幂等：第二次跑时 user_id 已经是 legacy 格式，LIKE '{%' 不中，UPDATE 0 行。

UPDATE channel_monitor_request_templates
SET body_override = jsonb_set(
        body_override,
        '{metadata,user_id}',
        '"user_0000000000000000000000000000000000000000000000000000000000000000_account_00000000-0000-0000-0000-000000000000_session_00000000-0000-0000-0000-000000000000"'::jsonb,
        false
    ),
    updated_at = NOW()
WHERE provider = 'anthropic'
  AND name = 'Claude Code 伪装'
  AND body_override #>> '{metadata,user_id}' LIKE '{%';

-- 同步已应用此模板的监控快照（监控采用 snapshot 语义，只更新那些明显还是 seed 原样的）。
UPDATE channel_monitors m
SET body_override = jsonb_set(
        m.body_override,
        '{metadata,user_id}',
        '"user_0000000000000000000000000000000000000000000000000000000000000000_account_00000000-0000-0000-0000-000000000000_session_00000000-0000-0000-0000-000000000000"'::jsonb,
        false
    )
FROM channel_monitor_request_templates t
WHERE m.template_id = t.id
  AND t.provider = 'anthropic'
  AND t.name = 'Claude Code 伪装'
  AND m.body_override #>> '{metadata,user_id}' LIKE '{%';
