-- 为 redeem_codes 表添加备注字段

ALTER TABLE redeem_codes
ADD COLUMN IF NOT EXISTS notes TEXT DEFAULT NULL;

COMMENT ON COLUMN redeem_codes.notes IS '备注说明（管理员调整时的原因说明）';
