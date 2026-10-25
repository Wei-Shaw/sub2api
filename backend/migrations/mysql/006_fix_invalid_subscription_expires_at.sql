-- MySQL 方言版本（翻译自 ../006_fix_invalid_subscription_expires_at.sql）
-- Fix legacy subscription records with invalid expires_at (year > 2099).
--
-- 翻译说明：PG DO 块（to_regclass 存在性守卫 + UPDATE）→ 直接执行语句体。
--           守卫可去掉：迁移按文件名顺序执行，003 已保证 user_subscriptions 表存在。
--           TIMESTAMPTZ '2099-12-31 23:59:59+00' → DATETIME(3) 字面量 '2099-12-31 23:59:59.000'。
UPDATE user_subscriptions
SET expires_at = '2099-12-31 23:59:59.000'
WHERE expires_at > '2099-12-31 23:59:59.000';
