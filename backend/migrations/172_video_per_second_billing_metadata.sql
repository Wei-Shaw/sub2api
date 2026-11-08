-- Grok video billing is per second of generated output (xAI rate card), so usage
-- rows must record the billed resolution and duration for auditability. The
-- image-size check constraint must also exempt any video row by video_count
-- instead of billing_mode='video' alone: a video request billed through a
-- token-mode channel price produces billing_mode='token' with image_count=1
-- (legacy media counter) and no image_size, which the previous constraint
-- rejected and silently dropped the whole billing transaction.

ALTER TABLE usage_logs
    ADD COLUMN IF NOT EXISTS video_count INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS video_resolution VARCHAR(10),
    ADD COLUMN IF NOT EXISTS video_duration_seconds INTEGER;

COMMENT ON COLUMN usage_logs.video_count IS '视频生成数量；>0 表示本行是视频生成用量';
COMMENT ON COLUMN usage_logs.video_resolution IS '计费用视频分辨率 480p/720p/1080p';
COMMENT ON COLUMN usage_logs.video_duration_seconds IS '提交时请求的视频时长（秒），按秒计费的乘数';

ALTER TABLE usage_logs
    DROP CONSTRAINT IF EXISTS usage_logs_image_billing_size_check;

ALTER TABLE usage_logs
    ADD CONSTRAINT usage_logs_image_billing_size_check
    CHECK (
        image_count <= 0
        OR billing_mode = 'video'
        OR COALESCE(video_count, 0) > 0
        OR (
            image_size IS NOT NULL
            AND image_size IN ('1K', '2K', '4K', 'mixed')
        )
    ) NOT VALID;

-- Group video prices are per-second rates (USD/s), matching the xAI rate card;
-- total cost = per-second price x duration seconds. Clarify the column docs
-- introduced by migration 170, which read as per-video prices.
COMMENT ON COLUMN groups.video_price_480p IS '480p 视频生成每秒单价 (USD/s)，Grok 平台使用';
COMMENT ON COLUMN groups.video_price_720p IS '720p 视频生成每秒单价 (USD/s)，Grok 平台使用';
COMMENT ON COLUMN groups.video_price_1080p IS '1080p 视频生成每秒单价 (USD/s)，Grok 平台使用';
