-- MySQL 方言版本（翻译自 ../219_group_search_price_per_1k.sql）
-- 翻译说明：`groups` 为 MySQL 8 保留字需反引号；ADD COLUMN IF NOT EXISTS → ADD COLUMN。

-- Grok / 通用搜索工具显式定价（per 1000 calls，USD）。
-- NULL = 使用代码默认 $10/1k；显式 0 = 免费；>0 = 分组覆盖价。
ALTER TABLE `groups` ADD COLUMN search_price_per_1k DECIMAL(20,8) NULL;
