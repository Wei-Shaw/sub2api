-- 退款类审计事件是“每次尝试一条”的追加记录：退款重试会再次写入同一 (order_id, action)。
-- latestRefundPendingDetail 也按 created_at/id 取最近一条 REFUND_PENDING，说明该表本来就允许
-- 同单多条 REFUND_PENDING。
--
-- 131 建立的全量唯一索引把这些重复写入变成约束错误，而退款收敛路径在事务内写审计：
-- 事务整体回滚，导致保留的余额/订阅天数不再归还、订单卡在 REFUND_PENDING/REFUNDING，
-- 重试退款时还可能再次扣减。REFUND_FAILED 的重复写入同理。
--
-- 因此把唯一约束收窄到真正“一单只能发生一次”的动作：订阅发放认领（SUBSCRIPTION_ASSIGNED，
-- 履约依赖约束错误做幂等）与返利发放幂等（AFFILIATE_REBATE_APPLIED/SKIPPED）。
-- 先并发建好新索引，再删除旧索引，迁移过程中唯一性保护不出现空窗。
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS idx_payment_audit_logs_order_action_once_uniq
    ON payment_audit_logs (order_id, action)
    WHERE action IN ('SUBSCRIPTION_ASSIGNED', 'AFFILIATE_REBATE_APPLIED', 'AFFILIATE_REBATE_SKIPPED');

DROP INDEX CONCURRENTLY IF EXISTS idx_payment_audit_logs_order_action_uniq;
