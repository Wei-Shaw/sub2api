-- MySQL 方言版本（翻译自 ../172_video_per_second_billing_metadata.sql）
-- 翻译说明：
--   - ADD COLUMN IF NOT EXISTS → ADD COLUMN；INTEGER → INT。
--   - COMMENT ON COLUMN → 整句跳过。
--   - DROP CONSTRAINT IF EXISTS（CHECK 约束）→ DROP CHECK；NOT VALID → 去掉
--     （MySQL 不支持，ADD CONSTRAINT 时即校验存量行）。原约束由 171 以同名重建。

-- Grok video billing is per second of generated output (xAI rate card), so usage
-- rows must record the billed resolution and duration for auditability. The
-- image-size check constraint must also exempt any video row by video_count
-- instead of billing_mode='video' alone: a video request billed through a
-- token-mode channel price produces billing_mode='token' with image_count=1
-- (legacy media counter) and no image_size, which the previous constraint
-- rejected and silently dropped the whole billing transaction.

ALTER TABLE usage_logs
    ADD COLUMN video_count INT NOT NULL DEFAULT 0,
    ADD COLUMN video_resolution VARCHAR(10) NULL,
    ADD COLUMN video_duration_seconds INT NULL;

-- (comment skipped)
-- (comment skipped)
-- (comment skipped)

ALTER TABLE usage_logs
    DROP CHECK usage_logs_image_billing_size_check;

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
    );

-- Group video prices are per-second rates (USD/s), matching the xAI rate card;
-- total cost = per-second price x duration seconds. Clarify the column docs
-- introduced by migration 170, which read as per-video prices.
-- (comment skipped)
-- (comment skipped)
-- (comment skipped)
