-- MySQL 方言版本（翻译自 ../134_affiliate_ledger_audit_snapshots.sql）
--
-- 翻译决策：
--   - ADD COLUMN IF NOT EXISTS → ADD COLUMN；内联 REFERENCES → 显式命名 FK。
--   - 部分索引均为非唯一：WHERE source_order_id IS NOT NULL / WHERE action = 'accrue'
--     → 按规则去掉 WHERE（普通索引，仅性能差异）。
--   - CROSS JOIN LATERAL（按行提取 detail 中 rebateAmount 数字）→ 内联表达式：
--     REGEXP_SUBSTR 取整段匹配 + REGEXP_REPLACE 剥离前缀 + CAST AS DECIMAL(20,10)
--     （MySQL 8 的 REGEXP_SUBSTR 不支持提取捕获组；无匹配返回 NULL，与 PG substring() 一致）。
--   - EXTRACT(EPOCH FROM (a - b)) → ABS(TIMESTAMPDIFF(MICROSECOND, b, a))（排序等价）。
--   - interval '10 minutes' → INTERVAL 10 MINUTE；po.id::text → CAST(po.id AS CHAR)。
--   - WITH ... UPDATE ... FROM + NOT EXISTS（目标表同时出现在派生表与 NOT EXISTS 子查询中，
--     MySQL 1093 限制）→ 临时存储过程：先把最终匹配集（含 NOT EXISTS 过滤）物化到
--     临时表（纯读），再 UPDATE ... JOIN 临时表回填；语义与 PG 单语句快照一致
--     （匹配集在回填前计算）。过程结束即 DROP（不写 DELIMITER）。

-- 邀请返利流水补充订单关联和转余额快照。
-- 这些字段只用于审计展示；历史旧流水无法可靠反推的字段保持 NULL，避免把当前状态误展示为历史状态。

ALTER TABLE user_affiliate_ledger
    ADD COLUMN source_order_id BIGINT NULL;

ALTER TABLE user_affiliate_ledger
    ADD CONSTRAINT fk_user_affiliate_ledger_source_order_id
    FOREIGN KEY (source_order_id) REFERENCES payment_orders(id) ON DELETE SET NULL;

ALTER TABLE user_affiliate_ledger
    ADD COLUMN balance_after DECIMAL(20,8) NULL;

ALTER TABLE user_affiliate_ledger
    ADD COLUMN aff_quota_after DECIMAL(20,8) NULL;

ALTER TABLE user_affiliate_ledger
    ADD COLUMN aff_frozen_quota_after DECIMAL(20,8) NULL;

ALTER TABLE user_affiliate_ledger
    ADD COLUMN aff_history_quota_after DECIMAL(20,8) NULL;

-- (comment skipped)
-- (comment skipped)
-- (comment skipped)
-- (comment skipped)
-- (comment skipped)

CREATE INDEX idx_user_affiliate_ledger_source_order_id
    ON user_affiliate_ledger(source_order_id);

CREATE INDEX idx_user_affiliate_ledger_rebate_lookup
    ON user_affiliate_ledger(action, source_order_id, user_id, source_user_id, created_at);

-- 尽力回填 PR #2169 合并后、该迁移前已经产生的返利流水。
-- 只有在同一订单只能匹配到一条返利流水时才回填，避免把多笔同额流水错误绑定到订单。
DROP PROCEDURE IF EXISTS __mig_134_ledger_tmp;
CREATE PROCEDURE __mig_134_ledger_tmp()
BEGIN
    CREATE TEMPORARY TABLE __mig_134_matches (
        ledger_id BIGINT NOT NULL,
        order_id  BIGINT NOT NULL
    ) ENGINE=InnoDB;

    INSERT INTO __mig_134_matches (ledger_id, order_id)
    WITH rebate_audits AS (
        SELECT po.id AS order_id,
               po.user_id AS invitee_user_id,
               invitee_aff.inviter_id,
               CAST(
                   REGEXP_REPLACE(
                       REGEXP_SUBSTR(
                           pal.detail,
                           '"rebateAmount"[[:space:]]*:[[:space:]]*-?[0-9]+(\\.[0-9]+)?',
                           1, 1, 'c'
                       ),
                       '^"rebateAmount"[[:space:]]*:[[:space:]]*',
                       ''
                   ) AS DECIMAL(20,10)
               ) AS rebate_amount,
               pal.created_at AS audit_created_at
        FROM payment_audit_logs pal
        JOIN payment_orders po ON CAST(po.id AS CHAR) = pal.order_id
        JOIN user_affiliates invitee_aff ON invitee_aff.user_id = po.user_id
        WHERE pal.action = 'AFFILIATE_REBATE_APPLIED'
    ),
    rebate_audits_filtered AS (
        SELECT *
        FROM rebate_audits
        WHERE rebate_amount IS NOT NULL
    ),
    ranked_matches AS (
        SELECT ual.id AS ledger_id,
               ra.order_id,
               COUNT(*) OVER (PARTITION BY ra.order_id) AS order_match_count,
               COUNT(*) OVER (PARTITION BY ual.id) AS ledger_match_count,
               ROW_NUMBER() OVER (
                   PARTITION BY ual.id
                   ORDER BY ABS(TIMESTAMPDIFF(MICROSECOND, ra.audit_created_at, ual.created_at)), ra.order_id
               ) AS ledger_rank
        FROM rebate_audits_filtered ra
        JOIN user_affiliate_ledger ual
          ON ual.action = 'accrue'
         AND ual.source_order_id IS NULL
         AND ual.user_id = ra.inviter_id
         AND ual.source_user_id = ra.invitee_user_id
         AND ABS(ual.amount - ra.rebate_amount) < 0.00000001
         AND ual.created_at BETWEEN ra.audit_created_at - INTERVAL 10 MINUTE
                                AND ra.audit_created_at + INTERVAL 10 MINUTE
    )
    SELECT rm.ledger_id, rm.order_id
    FROM ranked_matches rm
    WHERE rm.order_match_count = 1
      AND rm.ledger_match_count = 1
      AND rm.ledger_rank = 1
      AND NOT EXISTS (
          SELECT 1
          FROM user_affiliate_ledger existing
          WHERE existing.source_order_id = rm.order_id
            AND existing.action = 'accrue'
      );

    UPDATE user_affiliate_ledger AS ual
    JOIN __mig_134_matches AS m
      ON m.ledger_id = ual.id
    SET ual.source_order_id = m.order_id,
        ual.updated_at = NOW(3);

    DROP TEMPORARY TABLE __mig_134_matches;
END;

CALL __mig_134_ledger_tmp();
DROP PROCEDURE __mig_134_ledger_tmp;
