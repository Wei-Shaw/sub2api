-- MySQL 方言版本（翻译自 ../217_group_video_model_prices.sql）
-- 翻译说明：`groups` 为 MySQL 8 保留字需反引号；JSONB → JSON（可空）；
--           COMMENT ON COLUMN 整句跳过（规则文档约定）。

-- Per-model-family video per-second prices for Grok Imagine.
-- Shape: {"grok-imagine-video":{"480p":0.05,"720p":0.07},"grok-imagine-video-1.5":{"480p":0.08,"720p":0.14,"1080p":0.25}}
-- Resolution order in billing: per-model map → legacy video_price_* columns → code defaults (model-aware).
ALTER TABLE `groups`
    ADD COLUMN video_model_prices JSON NULL;

-- (comment skipped)
