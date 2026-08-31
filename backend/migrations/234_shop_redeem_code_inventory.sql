-- 商城商品与兑换码库存。
-- 商城兑换码是外部兑换码：购买后只标记为已售出，不调用本地 redeem_codes 兑换逻辑。
CREATE TABLE IF NOT EXISTS shop_products (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(200) NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    price NUMERIC(20, 8) NOT NULL CHECK (price >= 0),
    status VARCHAR(20) NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS shop_inventory_codes (
    id BIGSERIAL PRIMARY KEY,
    product_id BIGINT NOT NULL REFERENCES shop_products(id) ON DELETE CASCADE,
    code TEXT NOT NULL UNIQUE,
    status VARCHAR(20) NOT NULL DEFAULT 'available',
    sold_to BIGINT REFERENCES users(id) ON DELETE SET NULL,
    sold_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS shop_orders (
    id BIGSERIAL PRIMARY KEY,
    order_no VARCHAR(64) NOT NULL UNIQUE,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    product_id BIGINT NOT NULL REFERENCES shop_products(id) ON DELETE RESTRICT,
    product_name VARCHAR(200) NOT NULL,
    price NUMERIC(20, 8) NOT NULL,
    inventory_code_id BIGINT NOT NULL UNIQUE REFERENCES shop_inventory_codes(id) ON DELETE RESTRICT,
    status VARCHAR(20) NOT NULL DEFAULT 'paid',
    idempotency_key VARCHAR(128),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_shop_products_status ON shop_products(status);
CREATE INDEX IF NOT EXISTS idx_shop_inventory_codes_product_status ON shop_inventory_codes(product_id, status, id);
CREATE INDEX IF NOT EXISTS idx_shop_orders_user_created ON shop_orders(user_id, created_at DESC);
CREATE UNIQUE INDEX IF NOT EXISTS idx_shop_orders_user_idempotency ON shop_orders(user_id, idempotency_key) WHERE idempotency_key IS NOT NULL;
