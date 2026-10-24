-- 246_redeem_codes_batch_constraint.sql
-- 增加兑换码批次ID及单账号限兑一次字段与约束

ALTER TABLE redeem_codes ADD COLUMN IF NOT EXISTS batch_id VARCHAR(64);
ALTER TABLE redeem_codes ADD COLUMN IF NOT EXISTS only_once_per_account BOOLEAN NOT NULL DEFAULT FALSE;

-- 批次索引
CREATE INDEX IF NOT EXISTS idx_redeem_codes_batch_id ON redeem_codes(batch_id);

-- 单账号限兑一次部分唯一索引：同一用户在同一限兑批次下只能成功使用 1 张码
CREATE UNIQUE INDEX IF NOT EXISTS idx_redeem_codes_batch_user_once 
ON redeem_codes (batch_id, used_by) 
WHERE status = 'used' AND only_once_per_account = TRUE AND batch_id IS NOT NULL;
