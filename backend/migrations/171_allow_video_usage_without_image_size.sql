-- Grok video generation stores billing_mode='video' and keeps image_count=1
-- only as a legacy media-unit counter. It must not be forced to carry an
-- image_size, because video pricing uses video_resolution/request metadata.

ALTER TABLE usage_logs
    DROP CONSTRAINT IF EXISTS usage_logs_image_billing_size_check;

ALTER TABLE usage_logs
    ADD CONSTRAINT usage_logs_image_billing_size_check
    CHECK (
        image_count <= 0
        OR billing_mode = 'video'
        OR (
            image_size IS NOT NULL
            AND image_size IN ('1K', '2K', '4K', 'mixed')
        )
    ) NOT VALID;
