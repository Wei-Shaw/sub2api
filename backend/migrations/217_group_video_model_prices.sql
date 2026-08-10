-- Per-model-family video per-second prices for Grok Imagine.
-- Shape: {"grok-imagine-video":{"480p":0.05,"720p":0.07},"grok-imagine-video-1.5":{"480p":0.08,"720p":0.14,"1080p":0.25}}
-- Resolution order in billing: per-model map → legacy video_price_* columns → code defaults (model-aware).
ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS video_model_prices JSONB;
