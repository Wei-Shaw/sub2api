-- MySQL 方言版本（翻译自 ../053_add_security_secrets.sql）
-- 存储系统级密钥（如 JWT 签名密钥、TOTP 加密密钥）
-- 说明：`key` 在 MySQL 中是保留字（KEY），需加反引号。
CREATE TABLE IF NOT EXISTS security_secrets (
  id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
  `key` VARCHAR(100) NOT NULL UNIQUE,
  value TEXT NOT NULL,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE INDEX idx_security_secrets_key ON security_secrets (`key`);
