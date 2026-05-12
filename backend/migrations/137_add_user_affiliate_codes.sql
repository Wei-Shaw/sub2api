-- 邀请返利：邀请码事实表。
-- 业务约束：每个用户保留 1 个 manual 主邀请码；可选 1 个 auto-active companion 邀请码。
-- user_affiliates.aff_code 继续作为 manual 主邀请码/兼容字段；auto-active 码由主邀请码派生生成。

CREATE TABLE IF NOT EXISTS user_affiliate_codes (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    aff_code VARCHAR(32) NOT NULL UNIQUE,
    is_auto_active BOOLEAN NOT NULL DEFAULT false,
    is_primary BOOLEAN NOT NULL DEFAULT false,
    CONSTRAINT user_affiliate_codes_kind_check CHECK (is_primary <> is_auto_active),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_user_affiliate_codes_primary_or_auto
        CHECK (is_primary = true OR is_auto_active = true)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_user_affiliate_codes_one_primary
    ON user_affiliate_codes (user_id)
    WHERE is_primary = true;

CREATE UNIQUE INDEX IF NOT EXISTS idx_user_affiliate_codes_one_auto_active
    ON user_affiliate_codes (user_id)
    WHERE is_auto_active = true;

CREATE INDEX IF NOT EXISTS idx_user_affiliate_codes_user_id
    ON user_affiliate_codes (user_id, is_primary DESC, is_auto_active ASC, created_at ASC);

CREATE INDEX IF NOT EXISTS idx_user_affiliate_codes_auto_active
    ON user_affiliate_codes (is_auto_active)
    WHERE is_auto_active = true;

INSERT INTO user_affiliate_codes (user_id, aff_code, is_auto_active, is_primary, created_at, updated_at)
SELECT user_id, aff_code, false, true, created_at, updated_at
FROM user_affiliates
ON CONFLICT (aff_code) DO NOTHING;

COMMENT ON TABLE user_affiliate_codes IS '用户邀请码事实表；每个用户 1 个 manual 主码，可选 1 个 auto-active 派生码';
COMMENT ON COLUMN user_affiliate_codes.aff_code IS '邀请码，全局唯一';
COMMENT ON COLUMN user_affiliate_codes.is_auto_active IS '使用该邀请码 claim 设备时是否自动 active';
COMMENT ON COLUMN user_affiliate_codes.is_primary IS 'manual 主邀请码；同步到 user_affiliates.aff_code 作为兼容字段';
