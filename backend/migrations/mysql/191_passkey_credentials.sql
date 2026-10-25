-- MySQL 方言版本（翻译自 ../191_passkey_credentials.sql）
--
-- 说明：BYTEA → VARBINARY（MySQL 无 bytea；规则表未覆盖本项目首次出现，按 WebAuthn 规范上限取长度：
--       user_handle ≤64 字节取 VARBINARY(255)，credential_id ≤1023 字节取 VARBINARY(1023)，
--       VARBINARY 可直接建 UNIQUE 索引而 BLOB 不行）；PG 内联 REFERENCES → 显式命名 FK。
-- TODO-REVIEW: BYTEA → VARBINARY(255)/VARBINARY(1023) 长度为译者按 WebAuthn 规范上限的裁量
--       （规则表未定义 bytea 映射；应用层存取原始 []byte，见 backend/internal/repository/passkey_repo.go）。

CREATE TABLE IF NOT EXISTS passkey_user_handles (
    user_id BIGINT NOT NULL PRIMARY KEY,
    user_handle VARBINARY(255) NOT NULL UNIQUE,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    CONSTRAINT fk_passkey_user_handles_user_id FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS passkey_credentials (
    id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    user_id BIGINT NOT NULL,
    credential_id VARBINARY(1023) NOT NULL UNIQUE,
    name VARCHAR(100) NOT NULL DEFAULT 'Passkey',
    credential_data JSON NOT NULL,
    last_used_at DATETIME(3) NULL,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    CONSTRAINT fk_passkey_credentials_user_id FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE INDEX passkey_credentials_user_id_idx
    ON passkey_credentials (user_id);

CREATE INDEX passkey_credentials_last_used_at_idx
    ON passkey_credentials (last_used_at);
