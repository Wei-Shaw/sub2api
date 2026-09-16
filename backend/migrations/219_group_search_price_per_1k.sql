-- Grok / 通用搜索工具显式定价（per 1000 calls，USD）。
-- NULL = 使用代码默认 $10/1k；显式 0 = 免费；>0 = 分组覆盖价。
ALTER TABLE groups ADD COLUMN IF NOT EXISTS search_price_per_1k DECIMAL(20,8);
