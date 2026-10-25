-- MySQL 方言版本（翻译自 ../080_create_tls_fingerprint_profiles.sql）
-- Create tls_fingerprint_profiles table for managing TLS fingerprint templates.
-- Each profile contains ClientHello parameters to simulate specific client TLS handshake characteristics.

-- PG 的 SET LOCAL lock_timeout / statement_timeout 无 MySQL 等价语句，跳过。

CREATE TABLE IF NOT EXISTS tls_fingerprint_profiles (
    id           BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    name         VARCHAR(100) NOT NULL UNIQUE,
    description  TEXT,
    enable_grease BOOLEAN NOT NULL DEFAULT FALSE,
    cipher_suites        JSON,
    curves               JSON,
    point_formats        JSON,
    signature_algorithms JSON,
    alpn_protocols       JSON,
    supported_versions   JSON,
    key_share_groups     JSON,
    psk_modes            JSON,
    extensions           JSON,
    created_at   DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at   DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- (comment skipped) COMMENT ON TABLE tls_fingerprint_profiles
-- (comment skipped) COMMENT ON COLUMN tls_fingerprint_profiles.name
-- (comment skipped) COMMENT ON COLUMN tls_fingerprint_profiles.enable_grease
-- (comment skipped) COMMENT ON COLUMN tls_fingerprint_profiles.cipher_suites
-- (comment skipped) COMMENT ON COLUMN tls_fingerprint_profiles.extensions
