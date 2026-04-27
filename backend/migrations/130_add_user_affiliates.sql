CREATE TABLE IF NOT EXISTS user_affiliates (
    user_id BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    aff_code VARCHAR(32) NOT NULL UNIQUE,
    inviter_id BIGINT NULL REFERENCES users(id) ON DELETE SET NULL,
    aff_count INTEGER NOT NULL DEFAULT 0,
    aff_quota DECIMAL(20,8) NOT NULL DEFAULT 0,
    aff_history_quota DECIMAL(20,8) NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_user_affiliates_inviter_id ON user_affiliates(inviter_id);
CREATE INDEX IF NOT EXISTS idx_user_affiliates_aff_quota ON user_affiliates(aff_quota);

COMMENT ON TABLE user_affiliates IS '用户邀请返利信息';
COMMENT ON COLUMN user_affiliates.aff_code IS '用户邀请代码';
COMMENT ON COLUMN user_affiliates.inviter_id IS '邀请人用户ID';
COMMENT ON COLUMN user_affiliates.aff_count IS '累计邀请人数';
COMMENT ON COLUMN user_affiliates.aff_quota IS '当前可提取返利金额';
COMMENT ON COLUMN user_affiliates.aff_history_quota IS '累计返利历史金额';
