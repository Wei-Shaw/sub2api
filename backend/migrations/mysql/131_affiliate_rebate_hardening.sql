-- MySQL 方言版本（翻译自 ../131_affiliate_rebate_hardening.sql）
--
-- 翻译决策：
--   - value ~ 正则 → value REGEXP（见下方 TODO-REVIEW 关于转义语义的说明）。
--   - to_char((value::numeric * 100), 'FM999999990.########') →
--     TRIM(TRAILING '.' FROM TRIM(TRAILING '0' FROM CAST(... AS DECIMAL(30,8))))：
--     小数点后有效位保留、末尾零与小数点裁剪，与 PG FM/# 格式的直观输出（'0.2'→'20'、
--     '0.0025'→'0.25'、'1'→'100'）一致。
--   - CREATE TABLE 内联 REFERENCES → 显式命名 FK；BIGSERIAL → AUTO_INCREMENT。
--   - WITH ranked AS (...) DELETE ... USING → 单表 DELETE + 嵌套派生表（MySQL 1093 规避的
--     标准写法，窗口函数强制派生表物化后按 id 删除）。
--   - po.id::text → CAST(po.id AS CHAR)；NOW() → NOW(3)。
--   - COMMENT ON TABLE/COLUMN 整句跳过。

-- TODO-REVIEW: PG 原文正则为 '^-?[0-9]+(\\.[0-9]+)?$'（双反斜杠）。在 PostgreSQL 默认
-- TODO-REVIEW: standard_conforming_strings=on 下，该字面量传给正则引擎为 `\\.`（匹配「字面反斜杠
-- TODO-REVIEW: + 任意字符」），普通小数（如 '0.2'）根本不匹配，UPDATE 实际不会生效；作者意图显然是
-- TODO-REVIEW: `\.`` `（匹配小数点）。MySQL 侧按「作者意图」实现（REGEXP '^-?[0-9]+(\\.[0-9]+)?$'，
-- TODO-REVIEW: 其中 \\.` 在 MySQL 字符串字面量中即正则 `\.`=字面小数点）。若需严格复刻 PG 默认配置
-- TODO-REVIEW: 下的实际行为（仅匹配含反斜杠的值），应改用 REGEXP '^-?[0-9]+(\\\\.[0-9]+)?$'。

-- 1) Normalize historical affiliate rebate rate values.
-- Legacy compatibility treated 0<x<=1 as fractional inputs (e.g. 0.2 => 20%).
-- We now use pure percentage semantics, so convert persisted fractional values once.
UPDATE settings
SET value = TRIM(TRAILING '.' FROM TRIM(TRAILING '0' FROM CAST(CAST(value AS DECIMAL(20,10)) * 100 AS DECIMAL(30,8)))),
    updated_at = NOW(3)
WHERE `key` = 'affiliate_rebate_rate'
  AND value REGEXP '^-?[0-9]+(\\.[0-9]+)?$'
  AND CAST(value AS DECIMAL(20,10)) > 0
  AND CAST(value AS DECIMAL(20,10)) <= 1;

-- 2) Affiliate ledger for accrual/transfer traceability.
CREATE TABLE IF NOT EXISTS user_affiliate_ledger (
    id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    user_id BIGINT NOT NULL,
    action VARCHAR(32) NOT NULL,
    amount DECIMAL(20,8) NOT NULL,
    source_user_id BIGINT NULL,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    CONSTRAINT fk_user_affiliate_ledger_user_id FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    CONSTRAINT fk_user_affiliate_ledger_source_user_id FOREIGN KEY (source_user_id) REFERENCES users(id) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE INDEX idx_user_affiliate_ledger_user_id ON user_affiliate_ledger(user_id);
CREATE INDEX idx_user_affiliate_ledger_action ON user_affiliate_ledger(action);

-- (comment skipped)
-- (comment skipped)

-- 3) Enforce idempotency at DB layer for payment audit actions.
-- （PG: WITH ranked AS (...) DELETE ... USING；MySQL 1093 规避：窗口函数派生表物化后按 id 删除）
DELETE FROM payment_audit_logs
WHERE id IN (
    SELECT id
    FROM (
        SELECT id,
               ROW_NUMBER() OVER (PARTITION BY order_id, action ORDER BY id) AS rn
        FROM payment_audit_logs
    ) AS ranked
    WHERE ranked.rn > 1
);

CREATE UNIQUE INDEX idx_payment_audit_logs_order_action_uniq
ON payment_audit_logs(order_id, action);

-- 4) Prevent retroactive affiliate rebate issuance for legacy completed balance orders.
INSERT INTO payment_audit_logs (order_id, action, detail, operator, created_at)
SELECT CAST(po.id AS CHAR),
       'AFFILIATE_REBATE_SKIPPED',
       '{"reason":"baseline before affiliate rebate idempotency rollout"}',
       'system',
       NOW(3)
FROM payment_orders po
WHERE po.order_type = 'balance'
  AND po.status = 'COMPLETED'
  AND NOT EXISTS (
      SELECT 1
      FROM payment_audit_logs pal
      WHERE pal.order_id = CAST(po.id AS CHAR)
        AND pal.action IN ('AFFILIATE_REBATE_APPLIED', 'AFFILIATE_REBATE_SKIPPED')
  );
