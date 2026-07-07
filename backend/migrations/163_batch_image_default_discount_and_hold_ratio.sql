ALTER TABLE groups
    ALTER COLUMN batch_image_discount_multiplier SET DEFAULT 0.5,
    ALTER COLUMN batch_image_hold_multiplier SET DEFAULT 0.6;

UPDATE groups
SET batch_image_discount_multiplier = 0.5
WHERE batch_image_discount_multiplier = 1.0;

UPDATE groups
SET batch_image_hold_multiplier = 0.6
WHERE batch_image_hold_multiplier = 1.05;

COMMENT ON COLUMN groups.batch_image_hold_multiplier IS '批量图片生成冻结价格比例，按普通生图原价乘以该比例冻结，结算后释放差额';

ALTER TABLE batch_image_jobs
    ALTER COLUMN batch_discount_multiplier SET DEFAULT 0.5,
    ALTER COLUMN hold_multiplier SET DEFAULT 0.6;

COMMENT ON COLUMN batch_image_jobs.hold_multiplier IS '提交时快照的冻结价格比例，按普通生图原价乘以该比例冻结';
