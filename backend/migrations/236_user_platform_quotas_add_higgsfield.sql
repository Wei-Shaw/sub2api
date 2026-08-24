-- Allow Higgsfield accounts to use the same per-user platform quota controls
-- as the other asynchronous media providers.
ALTER TABLE user_platform_quotas
    DROP CONSTRAINT IF EXISTS user_platform_quotas_platform_check;

ALTER TABLE user_platform_quotas
    ADD CONSTRAINT user_platform_quotas_platform_check
    CHECK (platform IN ('anthropic', 'openai', 'gemini', 'antigravity', 'grok',
                        'kiro', 'fal', 'atlascloud', 'apiz', 'higgsfield',
                        'kimi', 'zhipu', 'deepseek'));
