-- feature issue #1750: 清理历史 daily_usage_limit_usd = 0 / 负数的脏数据。
--
-- 背景：Resolve 在 quota_service.go 中只在 *v > 0 时下发限额；之前允许用户显式写入 0，
-- 但 0 的语义不明确（是"禁止所有请求"还是"不限"），两种读法在不同版本之间切换会把用户卡死或放行。
-- 当前契约（DAILY_QUOTA_CONTRACT §0.3）规定 0 视同 NULL（跟随全局不限）。
-- 这里一次性把 0 及负值清掉，DB 只保留 {NULL, 正值} 两态，和写入路径归一化
-- （UpdateUserQuota 的 quotaMinPositiveLimit 阈值）对齐。
--
-- 幂等：UPDATE 条件 `<= 0` 再次执行会命中 0 条行（已清理过）。无 schema 变更，纯数据修复。

BEGIN;

UPDATE users
SET daily_usage_limit_usd = NULL
WHERE daily_usage_limit_usd IS NOT NULL
  AND daily_usage_limit_usd <= 0;

COMMIT;
