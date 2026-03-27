-- 077: Rollback Sonnet/Haiku mappings from Opus back to original values
-- Reverts migration 077_migrate_sonnet_to_opus46_thinking.sql
-- Restores: Sonnet→Sonnet, Haiku→Sonnet (original behavior)

UPDATE accounts
SET credentials = jsonb_set(
    credentials, '{model_mapping}',
    (credentials->'model_mapping')
        || '{"claude-sonnet-4-6": "claude-sonnet-4-6"}'::jsonb
        || '{"claude-sonnet-4-5": "claude-sonnet-4-5"}'::jsonb
        || '{"claude-sonnet-4-5-thinking": "claude-sonnet-4-5-thinking"}'::jsonb
        || '{"claude-sonnet-4-5-20250929": "claude-sonnet-4-5"}'::jsonb
        || '{"claude-haiku-4-5": "claude-sonnet-4-6"}'::jsonb
        || '{"claude-haiku-4-5-20251001": "claude-sonnet-4-6"}'::jsonb
)
WHERE platform = 'antigravity'
  AND deleted_at IS NULL
  AND credentials->'model_mapping' IS NOT NULL;
