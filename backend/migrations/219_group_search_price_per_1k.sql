-- Grok / 通用搜索工具显式定价（per 1000 calls，USD）。
ALTER TABLE groups ADD COLUMN IF NOT EXISTS search_price_per_1k DECIMAL(20,8);
