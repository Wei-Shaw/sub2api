-- MySQL 方言版本（翻译自 ../130_add_user_affiliates.sql）
-- 翻译说明：
--   - 内联 REFERENCES → 显式命名的 CONSTRAINT fk_xxx FOREIGN KEY。
--   - INTEGER → INT；TIMESTAMPTZ → DATETIME(3)，DEFAULT NOW() → DEFAULT CURRENT_TIMESTAMP(3)。
--   - CREATE INDEX IF NOT EXISTS → CREATE INDEX。
--   - COMMENT ON TABLE/COLUMN 整句跳过。

CREATE TABLE IF NOT EXISTS user_affiliates (
    user_id BIGINT NOT NULL PRIMARY KEY,
    aff_code VARCHAR(32) NOT NULL UNIQUE,
    inviter_id BIGINT NULL,
    aff_count INT NOT NULL DEFAULT 0,
    aff_quota DECIMAL(20,8) NOT NULL DEFAULT 0,
    aff_history_quota DECIMAL(20,8) NOT NULL DEFAULT 0,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    CONSTRAINT fk_user_affiliates_user_id FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    CONSTRAINT fk_user_affiliates_inviter_id FOREIGN KEY (inviter_id) REFERENCES users(id) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE INDEX idx_user_affiliates_inviter_id ON user_affiliates(inviter_id);
CREATE INDEX idx_user_affiliates_aff_quota ON user_affiliates(aff_quota);

-- (comment skipped)
-- (comment skipped)
-- (comment skipped)
-- (comment skipped)
-- (comment skipped)
-- (comment skipped)
