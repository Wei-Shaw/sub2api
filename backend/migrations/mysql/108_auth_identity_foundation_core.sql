-- MySQL 方言版本（翻译自 ../108_auth_identity_foundation_core.sql）
--
-- 翻译说明：
--   - TIMESTAMPTZ → DATETIME(3)；NOW() → CURRENT_TIMESTAMP(3)；JSONB → JSON
--     （DEFAULT '{}'::jsonb → DEFAULT ('{}')，8.0.13+ 表达式默认值）。
--   - ADD COLUMN IF NOT EXISTS / CREATE [UNIQUE] INDEX IF NOT EXISTS → 去掉
--     IF NOT EXISTS（执行器保证单次执行）。
--   - DO $$ 块（仅当 users_signup_source_check 不存在时才添加）→ 去掉存在性分支，
--     直接执行 ADD CONSTRAINT（约束名与 PG 同名保留；后续 135 依赖该名称 DROP CHECK）。
--   - 内联 REFERENCES → 显式命名的 CONSTRAINT fk_xxx FOREIGN KEY（便于后续 DROP FOREIGN KEY）。
--   - CHECK 约束同名保留（MySQL 8.0.16+ 强制执行并校验存量行；8.0.13~8.0.15 解析但不生效）。
--   - MySQL 的 TEXT 列不能整列参与索引 → 所有涉及索引的 TEXT 列
--     （provider_key / provider_subject / channel_app_id / channel_subject /
--     completion_code_hash / report_key）使用 (191) 前缀
--     （utf8mb4 下 191*4=764 字节，组合索引总长不超过 InnoDB 3072 字节上限）。
-- TODO-REVIEW: 上述 TEXT 前缀索引使 MySQL 侧的唯一性约束为「前缀唯一」（PG 为整值唯一）。
--   191 字符前缀完整覆盖 email 形态的 provider_subject（≤254 字符）与常规
--   issuer||sub 场景；若业务侧可能出现超过 191 字符且仅尾部不同的取值，
--   两者才会出现约束差异，需评估是否改用更长前缀或哈希列。

ALTER TABLE users
ADD COLUMN signup_source VARCHAR(20) NOT NULL DEFAULT 'email',
ADD COLUMN last_login_at DATETIME(3) NULL,
ADD COLUMN last_active_at DATETIME(3) NULL;

UPDATE users
SET signup_source = 'email'
WHERE signup_source IS NULL OR signup_source = '';

-- DO $$：users_signup_source_check 存在性检查去掉后直接添加（同名，135 依赖）
ALTER TABLE users
    ADD CONSTRAINT users_signup_source_check
    CHECK (signup_source IN ('email', 'linuxdo', 'wechat', 'oidc'));

CREATE TABLE IF NOT EXISTS auth_identities (
    id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    user_id BIGINT NOT NULL,
    provider_type VARCHAR(20) NOT NULL,
    provider_key TEXT NOT NULL,
    provider_subject TEXT NOT NULL,
    verified_at DATETIME(3) NULL,
    issuer TEXT NULL,
    metadata JSON NOT NULL DEFAULT ('{}'),
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    CONSTRAINT auth_identities_provider_type_check
        CHECK (provider_type IN ('email', 'linuxdo', 'wechat', 'oidc')),
    CONSTRAINT fk_auth_identities_user_id
        FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE UNIQUE INDEX auth_identities_provider_subject_key
    ON auth_identities (provider_type, provider_key(191), provider_subject(191));

CREATE INDEX auth_identities_user_id_idx
    ON auth_identities (user_id);

CREATE INDEX auth_identities_user_provider_idx
    ON auth_identities (user_id, provider_type);

CREATE TABLE IF NOT EXISTS auth_identity_channels (
    id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    identity_id BIGINT NOT NULL,
    provider_type VARCHAR(20) NOT NULL,
    provider_key TEXT NOT NULL,
    channel VARCHAR(20) NOT NULL,
    channel_app_id TEXT NOT NULL,
    channel_subject TEXT NOT NULL,
    metadata JSON NOT NULL DEFAULT ('{}'),
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    CONSTRAINT auth_identity_channels_provider_type_check
        CHECK (provider_type IN ('email', 'linuxdo', 'wechat', 'oidc')),
    CONSTRAINT fk_auth_identity_channels_identity_id
        FOREIGN KEY (identity_id) REFERENCES auth_identities(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE UNIQUE INDEX auth_identity_channels_channel_key
    ON auth_identity_channels (provider_type, provider_key(191), channel, channel_app_id(191), channel_subject(191));

CREATE INDEX auth_identity_channels_identity_id_idx
    ON auth_identity_channels (identity_id);

CREATE TABLE IF NOT EXISTS pending_auth_sessions (
    id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    session_token VARCHAR(255) NOT NULL,
    intent VARCHAR(40) NOT NULL,
    provider_type VARCHAR(20) NOT NULL,
    provider_key TEXT NOT NULL,
    provider_subject TEXT NOT NULL,
    target_user_id BIGINT NULL,
    redirect_to TEXT NOT NULL DEFAULT (''),
    resolved_email TEXT NOT NULL DEFAULT (''),
    registration_password_hash TEXT NOT NULL DEFAULT (''),
    upstream_identity_claims JSON NOT NULL DEFAULT ('{}'),
    local_flow_state JSON NOT NULL DEFAULT ('{}'),
    browser_session_key TEXT NOT NULL DEFAULT (''),
    completion_code_hash TEXT NOT NULL DEFAULT (''),
    completion_code_expires_at DATETIME(3) NULL,
    email_verified_at DATETIME(3) NULL,
    password_verified_at DATETIME(3) NULL,
    totp_verified_at DATETIME(3) NULL,
    expires_at DATETIME(3) NOT NULL,
    consumed_at DATETIME(3) NULL,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    CONSTRAINT pending_auth_sessions_intent_check
        CHECK (intent IN ('login', 'bind_current_user', 'adopt_existing_user_by_email')),
    CONSTRAINT pending_auth_sessions_provider_type_check
        CHECK (provider_type IN ('email', 'linuxdo', 'wechat', 'oidc')),
    CONSTRAINT fk_pending_auth_sessions_target_user_id
        FOREIGN KEY (target_user_id) REFERENCES users(id) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE UNIQUE INDEX pending_auth_sessions_session_token_key
    ON pending_auth_sessions (session_token);

CREATE INDEX pending_auth_sessions_target_user_id_idx
    ON pending_auth_sessions (target_user_id);

CREATE INDEX pending_auth_sessions_expires_at_idx
    ON pending_auth_sessions (expires_at);

CREATE INDEX pending_auth_sessions_provider_idx
    ON pending_auth_sessions (provider_type, provider_key(191), provider_subject(191));

CREATE INDEX pending_auth_sessions_completion_code_idx
    ON pending_auth_sessions (completion_code_hash(191));

CREATE TABLE IF NOT EXISTS identity_adoption_decisions (
    id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    pending_auth_session_id BIGINT NOT NULL,
    identity_id BIGINT NULL,
    adopt_display_name BOOLEAN NOT NULL DEFAULT FALSE,
    adopt_avatar BOOLEAN NOT NULL DEFAULT FALSE,
    decided_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    CONSTRAINT fk_identity_adoption_decisions_pending_auth_session_id
        FOREIGN KEY (pending_auth_session_id) REFERENCES pending_auth_sessions(id) ON DELETE CASCADE,
    CONSTRAINT fk_identity_adoption_decisions_identity_id
        FOREIGN KEY (identity_id) REFERENCES auth_identities(id) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE UNIQUE INDEX identity_adoption_decisions_pending_auth_session_id_key
    ON identity_adoption_decisions (pending_auth_session_id);

CREATE INDEX identity_adoption_decisions_identity_id_idx
    ON identity_adoption_decisions (identity_id);

CREATE TABLE IF NOT EXISTS auth_identity_migration_reports (
    id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    report_type VARCHAR(40) NOT NULL,
    report_key TEXT NOT NULL,
    details JSON NOT NULL DEFAULT ('{}'),
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE INDEX auth_identity_migration_reports_type_idx
    ON auth_identity_migration_reports (report_type);

CREATE UNIQUE INDEX auth_identity_migration_reports_type_key
    ON auth_identity_migration_reports (report_type, report_key(191));
