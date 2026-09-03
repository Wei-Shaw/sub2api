-- 194: 共享账号收益分配流水表
-- 记录 share_split / self_private_env 分账结果，便于对账。

CREATE TABLE IF NOT EXISTS share_revenue_ledgers (
    id                  BIGSERIAL PRIMARY KEY,
    request_id          VARCHAR(64) NOT NULL,
    usage_user_id       BIGINT NOT NULL,
    account_id          BIGINT,
    group_id            BIGINT,
    revenue_mode        VARCHAR(32) NOT NULL,
    total_cost          DECIMAL(20, 10) NOT NULL DEFAULT 0,
    billed_amount       DECIMAL(20, 10) NOT NULL DEFAULT 0,
    invite_amount       DECIMAL(20, 10) NOT NULL DEFAULT 0,
    user_amount         DECIMAL(20, 10) NOT NULL DEFAULT 0,
    platform_amount     DECIMAL(20, 10) NOT NULL DEFAULT 0,
    owner_user_id       BIGINT,
    inviter_user_id     BIGINT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_share_revenue_ledgers_request_id
    ON share_revenue_ledgers (request_id);

CREATE INDEX IF NOT EXISTS idx_share_revenue_ledgers_owner
    ON share_revenue_ledgers (owner_user_id, created_at DESC)
    WHERE owner_user_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_share_revenue_ledgers_usage_user
    ON share_revenue_ledgers (usage_user_id, created_at DESC);
