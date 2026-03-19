-- Map claude-haiku-4-5 variants target from claude-sonnet-4-5 to claude-sonnet-4-6
--
-- Only updates when the current target is exactly claude-sonnet-4-5.

-- 1. claude-haiku-4-5
UPDATE accounts
SET credentials = jsonb_set(credentials, '{model_mapping,claude-haiku-4-5REDACTED', '"claude-sonnet-4-6"')
WHERE platform = 'antigravity'
  AND deleted_at IS NULL
  AND credentials->'model_mapping'->>'claude-haiku-4-5' = 'claude-sonnet-4-5';

-- 2. REDACTED
UPDATE accounts
SET credentials = jsonb_set(credentials, '{model_mapping,REDACTEDREDACTED', '"claude-sonnet-4-6"')
WHERE platform = 'antigravity'
  AND deleted_at IS NULL
  AND credentials->'model_mapping'->>'REDACTED' = 'claude-sonnet-4-5';
