-- Migrate upstream accounts to apikey type
-- Background: upstream type is no longer needed. Antigravity platform APIKey accounts
-- with base_url pointing to an upstream sub2api instance can reuse the standard
-- APIKey forwarding path. GetBaseURL()/GetGeminiBaseURL() automatically appends
-- /antigravity for Antigravity platform APIKey accounts.

UPDATE accounts
SET type = 'apikey'
WHERE type = 'upstream'
  AND platform = 'antigravity'
  AND deleted_at IS NULL;
