-- Persist the image component of mixed Token + image billing for audit and UI display.

ALTER TABLE usage_logs
    ADD COLUMN IF NOT EXISTS image_generation_cost DECIMAL(20, 10) NOT NULL DEFAULT 0;

ALTER TABLE usage_logs
    ADD COLUMN IF NOT EXISTS image_actual_cost DECIMAL(20, 10) NOT NULL DEFAULT 0;

ALTER TABLE usage_logs
    ADD COLUMN IF NOT EXISTS image_rate_multiplier DECIMAL(10, 4) NOT NULL DEFAULT 0;
