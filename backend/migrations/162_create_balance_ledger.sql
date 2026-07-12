-- 余额 RPC：永久流水账本（钱的真值 + 审计），永不清理。
-- kind=1 deduct：amount 为扣费金额（正数），refunded_amount 累计已退。
-- kind=2 refund：amount 为退费金额（正数），refund_of 指向被冲销的原扣 request_id。
-- 幂等键 (app_id, request_id)：deduct 用调用方 request_id，refund 用调用方 refund_request_id。
-- amount/refunded_amount/balance_after 与 users.balance 对齐 decimal(20,8)。
CREATE TABLE IF NOT EXISTS balance_ledger (
    id              BIGSERIAL PRIMARY KEY,
    request_id      VARCHAR(128)   NOT NULL,
    app_id          VARCHAR(64)    NOT NULL,
    user_id         BIGINT         NOT NULL,
    kind            SMALLINT       NOT NULL,
    amount          DECIMAL(20,8)  NOT NULL,
    refunded_amount DECIMAL(20,8)  NOT NULL DEFAULT 0,
    refund_of       VARCHAR(128),
    description     TEXT           NOT NULL,
    extra           JSONB          NOT NULL DEFAULT '{}'::jsonb,
    balance_after   DECIMAL(20,8),
    created_at      TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ    NOT NULL DEFAULT NOW()
);

-- 扣/退幂等：同一 app 的 request_id 唯一。
CREATE UNIQUE INDEX IF NOT EXISTS balance_ledger_app_request_key ON balance_ledger (app_id, request_id);
-- 对账查询。
CREATE INDEX IF NOT EXISTS balance_ledger_user_created_idx ON balance_ledger (user_id, created_at);
-- 按原扣聚合退款。
CREATE INDEX IF NOT EXISTS balance_ledger_refund_of_idx ON balance_ledger (refund_of);
