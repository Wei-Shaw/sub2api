-- 商品名称、成交价已保存在订单快照中；允许删除下架商品但保留已交付订单与兑换码。
ALTER TABLE shop_orders DROP CONSTRAINT IF EXISTS shop_orders_product_id_fkey;
ALTER TABLE shop_orders ALTER COLUMN product_id DROP NOT NULL;
ALTER TABLE shop_orders ADD CONSTRAINT shop_orders_product_id_fkey
    FOREIGN KEY (product_id) REFERENCES shop_products(id) ON DELETE SET NULL;

ALTER TABLE shop_inventory_codes DROP CONSTRAINT IF EXISTS shop_inventory_codes_product_id_fkey;
ALTER TABLE shop_inventory_codes ALTER COLUMN product_id DROP NOT NULL;
ALTER TABLE shop_inventory_codes ADD CONSTRAINT shop_inventory_codes_product_id_fkey
    FOREIGN KEY (product_id) REFERENCES shop_products(id) ON DELETE SET NULL;
