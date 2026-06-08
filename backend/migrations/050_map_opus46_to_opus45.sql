-- Map claude-opus-4-6 to claude-opus-4-5-thinking
--
-- Notes:
-- - Updates existing Antigravity accounts' model_mapping
-- - Changes claude-opus-4-6 target from claude-opus-4-6 to claude-opus-4-5-thinking
-- - This is needed because previous versions didn't have this mapping

UPDATE accounts
SET credentials = jsonb_set(
    credentials,
    '{model_mapping,claude-opus-4-6}',
    '"claude-opus-4-5-thinking"'::jsonb
)
WHERE platform = 'antigravity'
  AND deleted_at IS NULL
  AND credentials->'model_mapping' IS NOT NULL
  AND credentials->'model_mapping'->>'claude-opus-4-6' IS NOT NULL;
