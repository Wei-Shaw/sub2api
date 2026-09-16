-- Per-model-family video per-second prices for Grok Imagine.
-- Shape: {"grok-imagine-video":{"480p":0.05,"720p":0.07},"grok-imagine-video-1.5":{"480p":0.08,"720p":0.14,"1080p":0.25}}
-- Resolution order in billing: per-model map → legacy video_price_* columns → code defaults (model-aware).
ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS video_model_prices JSONB;

COMMENT ON COLUMN groups.video_model_prices IS
    '可选：按模型族×分辨率覆盖视频每秒单价 (USD/s)。key 为规范模型族 (grok-imagine-video / grok-imagine-video-1.5)，value 为分辨率→单价映射；NULL/空表示不覆盖，回退到 video_price_* 列或官方默认';
