-- 记录邀请码的生成者：管理员生成的兑换码/邀请码为 NULL，
-- 普通用户通过「邀请好友」生成的邀请码记录生成者 user_id，用于按用户统计与限额。
ALTER TABLE redeem_codes
    ADD COLUMN IF NOT EXISTS created_by BIGINT;

COMMENT ON COLUMN redeem_codes.created_by IS '用户自助生成邀请码时的生成者 user_id；管理员生成的码为 NULL';

CREATE INDEX IF NOT EXISTS idx_redeem_codes_created_by
    ON redeem_codes (created_by)
    WHERE created_by IS NOT NULL;
