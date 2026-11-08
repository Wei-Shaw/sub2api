-- 月卡「每人限购」开关与次数（2026-11-06 主人要求）。
--
-- 背景：9.9 元那档是引流价，基本不赚钱，必须限制每个账号只能买一次；
-- 其余档位不限购。做成「开关 + 次数」两个字段而不是单个次数：
-- 开关关掉时前端把次数输入框也收起来（主人要求），后端也不用去猜 0 是「不限」还是「限 0 次」。
--
-- 判定口径：按 payment_orders(user_id, plan_id) 里**已支付**的订单数计数，
-- 因为 user_subscriptions 上没有 plan_id（一个分组可能对应多个套餐）。
ALTER TABLE subscription_plans
    ADD COLUMN IF NOT EXISTS purchase_limit_enabled BOOLEAN NOT NULL DEFAULT FALSE;

ALTER TABLE subscription_plans
    ADD COLUMN IF NOT EXISTS purchase_limit_count INTEGER NOT NULL DEFAULT 0;
