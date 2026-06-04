-- 充值赠送活动（recharge-bonus-promo）落库改动。
--
-- 1) 新增 recharge_promo_activities 表：每次 admin 保存活动配置都插入一行，
--    最新 created_at 即当前活动；用作前端红点 dismiss key 的稳定来源（version=自增 id）
--    及 admin "活动历史" tab 的数据源。硬删除策略，不带软删除字段。
-- 2) payment_orders 增加三列：
--      - bonus_amount  decimal(20,2) DEFAULT 0  —— 命中赠送时记录的赠送金额
--      - bonus_rate    decimal(10,4) DEFAULT 0  —— 命中档位的赠送比率（快照）
--      - activity_id   bigint NULL              —— 命中活动行的弱引用（无外键约束，便于历史清理）
--
-- 这是 forward-only migration；事务内执行（普通 DDL，无并发索引）。

CREATE TABLE IF NOT EXISTS recharge_promo_activities (
    id BIGSERIAL PRIMARY KEY,
    enabled BOOLEAN NOT NULL DEFAULT FALSE,
    valid_from TIMESTAMPTZ,
    valid_until TIMESTAMPTZ,
    tiers JSONB NOT NULL,
    operator VARCHAR(100) NOT NULL DEFAULT 'system',
    note TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS rechargepromoactivity_created_at
    ON recharge_promo_activities (created_at);

ALTER TABLE payment_orders
    ADD COLUMN IF NOT EXISTS bonus_amount DECIMAL(20,2) NOT NULL DEFAULT 0;

ALTER TABLE payment_orders
    ADD COLUMN IF NOT EXISTS bonus_rate DECIMAL(10,4) NOT NULL DEFAULT 0;

ALTER TABLE payment_orders
    ADD COLUMN IF NOT EXISTS activity_id BIGINT;
