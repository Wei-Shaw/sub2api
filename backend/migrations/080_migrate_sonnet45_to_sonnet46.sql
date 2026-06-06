-- 080: Migrate Sonnet 4.5 → Sonnet 4.6 for all antigravity accounts
-- Sonnet 4.5 is no longer reliable on upstream, redirect to 4.6

UPDATE accounts
SET credentials = jsonb_set(
    credentials, '{model_mapping}',
    (credentials->'model_mapping')
        || '{"claude-sonnet-4-5": "claude-sonnet-4-6"}'::jsonb
        || '{"claude-sonnet-4-5-thinking": "claude-sonnet-4-6"}'::jsonb
        || '{"claude-sonnet-4-5-20250929": "claude-sonnet-4-6"}'::jsonb
        || '{"claude-haiku-4-5": "claude-sonnet-4-6"}'::jsonb
        || '{"claude-haiku-4-5-20251001": "claude-sonnet-4-6"}'::jsonb
)
WHERE platform = 'antigravity'
  AND deleted_at IS NULL
  AND credentials->'model_mapping' IS NOT NULL;
