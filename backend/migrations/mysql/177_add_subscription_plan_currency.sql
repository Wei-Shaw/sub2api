-- MySQL 方言版本（翻译自 ../177_add_subscription_plan_currency.sql）
-- 翻译说明：ADD COLUMN IF NOT EXISTS → ADD COLUMN（IF NOT EXISTS 按迁移执行器单次执行语义去掉）。

-- Display-only ISO 4217 currency label for subscription plan prices; empty
-- keeps existing plans rendering without any label.
ALTER TABLE subscription_plans
    ADD COLUMN currency VARCHAR(3) NOT NULL DEFAULT '';
