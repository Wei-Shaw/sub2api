-- MySQL 方言版本（翻译自 ../160_add_user_frozen_balance.sql）
-- 翻译说明：ADD COLUMN IF NOT EXISTS → ADD COLUMN（IF NOT EXISTS 按迁移执行器单次执行语义去掉）。

ALTER TABLE users
    ADD COLUMN frozen_balance DECIMAL(20,8) NOT NULL DEFAULT 0;
