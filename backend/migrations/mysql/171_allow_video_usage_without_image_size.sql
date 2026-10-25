-- MySQL 方言版本（翻译自 ../171_allow_video_usage_without_image_size.sql）
-- 翻译说明：
--   - DROP CONSTRAINT IF EXISTS（CHECK 约束）→ ALTER TABLE ... DROP CHECK（MySQL 8.0.16+
--     命名 CHECK；IF EXISTS 按迁移执行器单次执行语义去掉）。原约束由 136_usage_log_image_size_metadata
--     以同名 ADD CONSTRAINT 创建。
--   - NOT VALID → MySQL 不支持，去掉；ADD CONSTRAINT 时即校验存量行（语义为最终一致）。

-- Grok video generation stores billing_mode='video' and keeps image_count=1
-- only as a legacy media-unit counter. It must not be forced to carry an
-- image_size, because video pricing uses video_resolution/request metadata.

ALTER TABLE usage_logs
    DROP CHECK usage_logs_image_billing_size_check;

ALTER TABLE usage_logs
    ADD CONSTRAINT usage_logs_image_billing_size_check
    CHECK (
        image_count <= 0
        OR billing_mode = 'video'
        OR (
            image_size IS NOT NULL
            AND image_size IN ('1K', '2K', '4K', 'mixed')
        )
    );
