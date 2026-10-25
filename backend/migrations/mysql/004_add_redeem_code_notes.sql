-- MySQL 方言版本（翻译自 ../004_add_redeem_code_notes.sql）
-- 为 redeem_codes 表添加备注字段

ALTER TABLE redeem_codes
ADD COLUMN notes TEXT DEFAULT NULL;

-- (comment skipped) COMMENT ON COLUMN redeem_codes.notes IS '备注说明（管理员调整时的原因说明）'
