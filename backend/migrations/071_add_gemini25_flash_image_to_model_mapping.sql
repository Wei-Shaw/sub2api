-- Add gemini-2.5-flash-image aliases to Antigravity model_mapping
--
-- Background:
-- Gemini native image generation now relies on gemini-2.5-flash-image, and
-- existing Antigravity accounts with persisted model_mapping need this alias in
-- order to participate in mixed scheduling from gemini groups.
--
-- Strategy:
-- Overwrite the stored model_mapping so it matches DefaultAntigravityModelMapping
-- in constants.go, including legacy gemini-3-pro-image aliases.

UPDATE accounts
SET credentials = jsonb_set(
    credentials,
    '{model_mapping}',
    '{
        "claude-opus-4-6-thinking": "claude-opus-4-6-thinking",
        "claude-opus-4-6": "claude-opus-4-6-thinking",
        "claude-opus-4-5-thinking": "claude-opus-4-6-thinking",
        "claude-opus-4-5-20251101": "claude-opus-4-6-thinking",
        "claude-sonnet-4-6": "claude-sonnet-4-6",
        "claude-sonnet-4-5": "claude-sonnet-4-5",
        "claude-sonnet-4-5-thinking": "claude-sonnet-4-5-thinking",
        "claude-sonnet-4-5-20250929": "claude-sonnet-4-5",
        "claude-haiku-4-5": "claude-sonnet-4-5",
        "claude-haiku-4-5-20251001": "claude-sonnet-4-5",
        "gemini-2.5-flash": "gemini-2.5-flash",
        "gemini-2.5-flash-image": "gemini-2.5-flash-image",
        "gemini-2.5-flash-image-preview": "gemini-2.5-flash-image",
        "gemini-2.5-flash-lite": "gemini-2.5-flash-lite",
        "gemini-2.5-flash-thinking": "gemini-2.5-flash-thinking",
        "gemini-2.5-pro": "gemini-2.5-pro",
        "gemini-3-flash": "gemini-3-flash",
        "gemini-3-pro-high": "gemini-3-pro-high",
        "gemini-3-pro-low": "gemini-3-pro-low",
        "gemini-3-flash-preview": "gemini-3-flash",
        "gemini-3-pro-preview": "gemini-3-pro-high",
        "gemini-3.1-pro-high": "gemini-3.1-pro-high",
        "gemini-3.1-pro-low": "gemini-3.1-pro-low",
        "gemini-3.1-pro-preview": "gemini-3.1-pro-high",
        "gemini-3.1-flash-image": "gemini-3.1-flash-image",
        "gemini-3.1-flash-image-preview": "gemini-3.1-flash-image",
        "gemini-3-pro-image": "gemini-3.1-flash-image",
        "gemini-3-pro-image-preview": "gemini-3.1-flash-image",
        "gpt-oss-120b-medium": "gpt-oss-120b-medium",
        "tab_flash_lite_preview": "tab_flash_lite_preview"
    }'::jsonb
)
WHERE platform = 'antigravity'
  AND deleted_at IS NULL
  AND credentials->'model_mapping' IS NOT NULL;
