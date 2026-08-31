-- 单用户限购；库存状态允许管理员将未售库存标记为已作废。
ALTER TABLE shop_products
    ADD COLUMN IF NOT EXISTS limit_per_user INTEGER NULL CHECK (limit_per_user IS NULL OR limit_per_user > 0);
CREATE INDEX IF NOT EXISTS idx_shop_orders_product_user ON shop_orders(product_id, user_id);
