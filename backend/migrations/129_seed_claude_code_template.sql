-- Migration: 129_seed_claude_code_template
-- 内置「Claude Code 伪装」请求模板，覆盖 Anthropic 上游对官方 CLI 客户端的所有验证项：
--   1) User-Agent / X-App / anthropic-beta / anthropic-version 等头
--   2) system 数组首项与官方 system prompt 字面一致（Dice >= 0.5）
--   3) metadata.user_id 满足 ParseMetadataUserID — 这里用 legacy 格式（user_<64hex>_account_<uuid>_session_<36char>）
--      避免新版 JSON 字符串内嵌 JSON 在编辑器里出现一长串 \" 转义，便于用户阅读。
--
-- ON CONFLICT DO NOTHING：已部署环境（手动建过模板）跑此 migration 不会重复 / 覆盖。
-- 用户可自行编辑后续覆盖此 seed；CC 升大版时再起一条 migration 提供新模板，不动用户的旧模板。

INSERT INTO channel_monitor_request_templates (
    name, provider, description, extra_headers, body_override_mode, body_override
)
VALUES (
    'Claude Code 伪装',
    'anthropic',
    '完整模拟 Claude Code 2.1.114 客户端：UA + anthropic-beta + system + metadata.user_id 全部对齐，绕过 Anthropic 上游 ''Claude Code only'' 限制（如 Max 套餐）。',
    '{
        "User-Agent": "claude-cli/2.1.114 (external, sdk-cli)",
        "X-App": "cli",
        "anthropic-version": "2023-06-01",
        "anthropic-beta": "claude-code-20250219,interleaved-thinking-2025-05-14,context-management-2025-06-27,prompt-caching-scope-2026-01-05,advisor-tool-2026-03-01",
        "anthropic-dangerous-direct-browser-access": "true"
    }'::jsonb,
    'merge',
    '{
        "system": [
            {
                "type": "text",
                "text": "You are Claude Code, Anthropic''s official CLI for Claude."
            }
        ],
        "metadata": {
            "user_id": "user_0000000000000000000000000000000000000000000000000000000000000000_account_00000000-0000-0000-0000-000000000000_session_00000000-0000-0000-0000-000000000000"
        }
    }'::jsonb
)
ON CONFLICT (provider, name) DO NOTHING;
