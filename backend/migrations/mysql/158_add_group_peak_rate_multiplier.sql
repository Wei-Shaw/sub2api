-- MySQL 方言版本（翻译自 ../158_add_group_peak_rate_multiplier.sql）
-- 翻译说明：`groups` 为 MySQL 8 保留字加反引号；ADD COLUMN IF NOT EXISTS → ADD COLUMN
-- （按迁移执行器单次执行语义去掉）。

ALTER TABLE `groups` ADD COLUMN peak_rate_enabled BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE `groups` ADD COLUMN peak_start VARCHAR(5) NOT NULL DEFAULT '';
ALTER TABLE `groups` ADD COLUMN peak_end VARCHAR(5) NOT NULL DEFAULT '';
ALTER TABLE `groups` ADD COLUMN peak_rate_multiplier DECIMAL(10,4) NOT NULL DEFAULT 1.0;
