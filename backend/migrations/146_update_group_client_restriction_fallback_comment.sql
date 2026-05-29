-- Expand fallback_group_id documentation: this field is now shared by
-- Anthropic Claude Code and OpenAI Codex official-client restrictions.

COMMENT ON COLUMN groups.fallback_group_id IS '客户端限制未命中时降级使用的分组 ID（Claude Code / OpenAI Codex）';
