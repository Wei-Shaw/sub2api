ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS batch_image_discount_multiplier DECIMAL(10,4) NOT NULL DEFAULT 0.5,
    ADD COLUMN IF NOT EXISTS batch_image_hold_multiplier DECIMAL(10,4) NOT NULL DEFAULT 0.6;

COMMENT ON COLUMN groups.batch_image_discount_multiplier IS '批量图片生成折扣倍率，最终单价会乘以该值；0 表示免费';
COMMENT ON COLUMN groups.batch_image_hold_multiplier IS '批量图片生成冻结价格比例，按普通生图原价乘以该比例冻结，结算后释放差额';

ALTER TABLE batch_image_jobs
    ADD COLUMN IF NOT EXISTS base_unit_price DECIMAL(20,10) NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS group_rate_multiplier DECIMAL(10,4) NOT NULL DEFAULT 1.0,
    ADD COLUMN IF NOT EXISTS account_rate_multiplier DECIMAL(10,4) NOT NULL DEFAULT 1.0,
    ADD COLUMN IF NOT EXISTS batch_discount_multiplier DECIMAL(10,4) NOT NULL DEFAULT 0.5,
    ADD COLUMN IF NOT EXISTS hold_multiplier DECIMAL(10,4) NOT NULL DEFAULT 0.6,
    ADD COLUMN IF NOT EXISTS billable_unit_price DECIMAL(20,10) NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS hold_unit_price DECIMAL(20,10) NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS pricing_snapshot_version INTEGER NOT NULL DEFAULT 0;

COMMENT ON COLUMN batch_image_jobs.base_unit_price IS '提交时快照的基础批量图片单价';
COMMENT ON COLUMN batch_image_jobs.group_rate_multiplier IS '提交时快照的分组/用户专属图片倍率';
COMMENT ON COLUMN batch_image_jobs.account_rate_multiplier IS '提交时快照的账号倍率';
COMMENT ON COLUMN batch_image_jobs.batch_discount_multiplier IS '提交时快照的批量折扣倍率';
COMMENT ON COLUMN batch_image_jobs.hold_multiplier IS '提交时快照的冻结价格比例，按普通生图原价乘以该比例冻结';
COMMENT ON COLUMN batch_image_jobs.billable_unit_price IS '提交时快照的实际结算单价';
COMMENT ON COLUMN batch_image_jobs.hold_unit_price IS '提交时快照的冻结单价';
COMMENT ON COLUMN batch_image_jobs.pricing_snapshot_version IS '批量图片任务价格快照版本；0 表示旧任务无快照';
