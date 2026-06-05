-- Force set default Antigravity model_mapping.
--
-- Notes:
-- - Applies to both Antigravity OAuth and Upstream accounts.
-- - Overwrites existing credentials.model_mapping.
-- - Removes legacy credentials.model_whitelist.

UPDATE accounts
SET credentials = (COALESCE(credentials, '{}'::jsonb) - 'model_whitelist' - 'model_mapping') || '{
  "model_mapping": {
    "claude-opus-4-6": "claude-opus-4-6",
    "claude-opus-4-5-thinking": "claude-opus-4-5-thinking",
    "claude-opus-4-5-20251101": "claude-opus-4-5-thinking",
    "claude-sonnet-4-5": "claude-sonnet-4-5",
    "claude-sonnet-4-5-thinking": "claude-sonnet-4-5-thinking",
    "claude-sonnet-4-5-20250929": "claude-sonnet-4-5",
    "claude-haiku-4-5": "claude-sonnet-4-5",
    "claude-haiku-4-5-20251001": "claude-sonnet-4-5",
    "gemini-2.5-flash": "gemini-2.5-flash",
    "gemini-2.5-flash-lite": "gemini-2.5-flash-lite",
    "gemini-2.5-flash-thinking": "gemini-2.5-flash-thinking",
    "gemini-2.5-pro": "gemini-2.5-pro",
    "gemini-3-flash": "gemini-3-flash",
    "gemini-3-flash-preview": "gemini-3-flash",
    "gemini-3-pro-high": "gemini-3-pro-high",
    "gemini-3-pro-low": "gemini-3-pro-low",
    "gemini-3-pro-image": "gemini-3-pro-image",
    "gemini-3-pro-preview": "gemini-3-pro-high",
    "gemini-3-pro-image-preview": "gemini-3-pro-image",
    "gpt-oss-120b-medium": "gpt-oss-120b-medium",
    "tab_flash_lite_preview": "tab_flash_lite_preview"
  }
}'::jsonb
WHERE platform = 'antigravity'
  AND deleted_at IS NULL;

