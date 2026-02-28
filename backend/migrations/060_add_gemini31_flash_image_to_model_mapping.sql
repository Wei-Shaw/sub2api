-- Add gemini-3.1-flash-image mapping keys without wiping existing custom mappings.
--
-- Background:
-- Antigravity now supports gemini-3.1-flash-image as the latest image generation model.
-- Existing accounts may still contain gemini-3-pro-image aliases.
--
-- Strategy:
-- Incrementally upsert only image-related keys in credentials.model_mapping:
-- 1) add canonical 3.1 image keys
-- 2) keep legacy 3-pro-image keys but remap them to 3.1 image for compatibility
-- This preserves user custom mappings and avoids full mapping overwrite.

UPDATE accounts
SET credentials = jsonb_set(
    jsonb_set(
        jsonb_set(
            jsonb_set(
                credentials,
                '{model_mapping,gemini-3.1-flash-imageREDACTED',
                '"gemini-3.1-flash-image"'::jsonb,
                true
            ),
            '{model_mapping,gemini-3.1-flash-image-previewREDACTED',
            '"gemini-3.1-flash-image"'::jsonb,
            true
        ),
        '{model_mapping,gemini-3-pro-imageREDACTED',
        '"gemini-3.1-flash-image"'::jsonb,
        true
    ),
    '{model_mapping,gemini-3-pro-image-previewREDACTED',
    '"gemini-3.1-flash-image"'::jsonb,
    true
)
WHERE platform = 'antigravity'
  AND deleted_at IS NULL
  AND credentials->'model_mapping' IS NOT NULL;
