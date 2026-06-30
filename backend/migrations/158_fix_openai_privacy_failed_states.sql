-- Backfill historical OpenAI OAuth privacy attempts that reached the desired
-- local scheduling state but were stored as retryable failures.
UPDATE accounts
SET extra = jsonb_set(COALESCE(extra, '{}'::jsonb), '{privacy_mode}', '"training_off"', true),
    updated_at = NOW()
WHERE platform = 'openai'
  AND type = 'oauth'
  AND extra->>'privacy_mode' IN ('training_set_failed', 'training_set_cf_blocked');
