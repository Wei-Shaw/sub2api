-- MySQL 方言版本（翻译自 ../033_add_promo_codes.sql）

-- 创建注册优惠码表
CREATE TABLE IF NOT EXISTS promo_codes (
    id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    code VARCHAR(32) NOT NULL UNIQUE,
    bonus_amount DECIMAL(20,8) NOT NULL DEFAULT 0,
    max_uses INT NOT NULL DEFAULT 0,
    used_count INT NOT NULL DEFAULT 0,
    status VARCHAR(20) NOT NULL DEFAULT 'active',
    expires_at DATETIME(3) DEFAULT NULL,
    notes TEXT NULL,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- 创建优惠码使用记录表
CREATE TABLE IF NOT EXISTS promo_code_usages (
    id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    promo_code_id BIGINT NOT NULL,
    user_id BIGINT NOT NULL,
    bonus_amount DECIMAL(20,8) NOT NULL,
    used_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    UNIQUE KEY uq_promo_code_usages_promo_code_id_user_id (promo_code_id, user_id),
    CONSTRAINT fk_promo_code_usages_promo_code_id FOREIGN KEY (promo_code_id) REFERENCES promo_codes(id) ON DELETE CASCADE,
    CONSTRAINT fk_promo_code_usages_user_id FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- 索引
CREATE INDEX idx_promo_codes_status ON promo_codes(status);
CREATE INDEX idx_promo_codes_expires_at ON promo_codes(expires_at);
CREATE INDEX idx_promo_code_usages_promo_code_id ON promo_code_usages(promo_code_id);
CREATE INDEX idx_promo_code_usages_user_id ON promo_code_usages(user_id);

-- (comment skipped)
-- (comment skipped)
-- (comment skipped)
-- (comment skipped)
