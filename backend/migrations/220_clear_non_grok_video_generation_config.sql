-- Videos are Grok/xAI-only. Clear stale video pricing from non-Grok groups.
-- Columns match migrations 170/217 (video_price_* / video_model_prices), not a
-- separate allow_video_generation flag which was never applied on this branch.

UPDATE groups
SET video_price_480p = NULL,
    video_price_720p = NULL,
    video_price_1080p = NULL,
    video_model_prices = NULL
WHERE platform IS DISTINCT FROM 'grok'
  AND (
      video_price_480p IS NOT NULL
      OR video_price_720p IS NOT NULL
      OR video_price_1080p IS NOT NULL
      OR video_model_prices IS NOT NULL
  );
