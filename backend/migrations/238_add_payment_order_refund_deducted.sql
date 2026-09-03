-- 退款扣减台账：记录订单上"已经扣走、尚未回滚"的余额与订阅天数。
--
-- 背景（H2）：ExecuteRefund 先从用户余额扣回退款额再调网关。网关失败时
-- handleGwFail 尝试 RollbackRefund；回滚也失败时订单落在 REFUND_FAILED，
-- 而 REFUND_FAILED 仍在 PrepareRefund 允许的重试状态里，prepDeduct 又不查
-- 历史扣减，于是管理员在后台再点一次"退款"就会把同一笔钱扣第二遍。
--
-- 这两列把"扣了多少还没还回去"落到订单行上，prepDeduct 据此只补扣差额。
-- 放在订单行而不是独立流水表，是因为退款状态机（ExecuteRefund 的
-- status CAS 到 REFUNDING）已经保证同一订单同时只有一个退款执行者，
-- 台账与状态同行可以被同一把行锁/同一次 CAS 覆盖；叙事性的历史留痕
-- 已经由 payment_audit_logs 承担，无需再开一张流水表。

ALTER TABLE payment_orders
    ADD COLUMN IF NOT EXISTS refund_deducted_amount DECIMAL(20,2) NOT NULL DEFAULT 0;

ALTER TABLE payment_orders
    ADD COLUMN IF NOT EXISTS refund_deducted_sub_days INTEGER NOT NULL DEFAULT 0;

COMMENT ON COLUMN payment_orders.refund_deducted_amount IS
    'Balance already deducted for this order refund and not yet rolled back';
COMMENT ON COLUMN payment_orders.refund_deducted_sub_days IS
    'Subscription days already deducted for this order refund and not yet rolled back';

-- ── 回填 ──────────────────────────────────────────────────────────────
--
-- 回填只认一个信号：payment_audit_logs 里最近一条 REFUND_ROLLBACK_FAILED。
-- 这正是现有代码判断"上一次扣减还挂着"所用的同一个信号
-- （payment_refund.go 里的 hasAuditLog(orderID, "REFUND_ROLLBACK_FAILED")），
-- 本迁移只是把那个布尔判断如实翻译成金额，不引入任何新的假设。
--
-- 为什么不按状态一刀切：
--   * 全部回填 0 —— 等于把线上已有的 REFUND_FAILED 订单继续留在重复扣款里；
--   * 对 REFUND_FAILED 一律回填 refund_amount —— 会把"回滚其实成功了"的订单
--     （网关失败 + 回滚成功的订单会被 restoreStatus 改回 COMPLETED，但经
--     QueryAndFinalizeRefund → finalizeRefundFailed 落到 REFUND_FAILED 的
--     订单确实可能已经回滚干净）错记成"已扣"，重试时少扣一笔，平台白送钱。
--   两种一刀切都会静默地把钱算错，所以只回填有确凿审计证据的那部分。
--
-- 已知残留风险（明确接受）：若当初余额回滚失败、连审计日志也没写成功，
-- 这里查不到证据，该订单回填 0，重试时仍会重复扣一次——与今天的行为完全
-- 一致，不比现状更差，且这类订单可由 [CRITICAL] rollback failed 日志人工核对。
--
-- 幂等：WHERE 只命中仍为 0 的行，重复执行结果相同；单行 JSON 解析失败
-- 时跳过该行而不是整个迁移失败。
DO $$
DECLARE
    rec RECORD;
    parsed JSONB;
    deducted_amount NUMERIC;
    deducted_days INTEGER;
BEGIN
    IF to_regclass('public.payment_audit_logs') IS NULL THEN
        RETURN;
    END IF;

    FOR rec IN
        SELECT DISTINCT ON (l.order_id) l.order_id, l.detail
        FROM payment_audit_logs l
        WHERE l.action = 'REFUND_ROLLBACK_FAILED'
          AND l.order_id ~ '^[0-9]+$'
        ORDER BY l.order_id, l.created_at DESC
    LOOP
        BEGIN
            parsed := rec.detail::jsonb;
        EXCEPTION WHEN others THEN
            CONTINUE;
        END;

        IF parsed IS NULL OR jsonb_typeof(parsed) <> 'object' THEN
            CONTINUE;
        END IF;

        deducted_amount := 0;
        deducted_days := 0;

        IF jsonb_typeof(parsed -> 'balanceDeducted') = 'number' THEN
            deducted_amount := (parsed ->> 'balanceDeducted')::numeric;
        END IF;
        IF jsonb_typeof(parsed -> 'subDaysDeducted') = 'number' THEN
            deducted_days := (parsed ->> 'subDaysDeducted')::numeric;
        END IF;

        IF deducted_amount <= 0 AND deducted_days <= 0 THEN
            CONTINUE;
        END IF;

        UPDATE payment_orders o
        SET refund_deducted_amount = GREATEST(deducted_amount, 0),
            refund_deducted_sub_days = GREATEST(deducted_days, 0)
        WHERE o.id = rec.order_id::bigint
          AND o.refund_deducted_amount = 0
          AND o.refund_deducted_sub_days = 0;
    END LOOP;
END
$$;
