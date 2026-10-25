-- MySQL 方言版本（翻译自 ../243_gemini_cached_contents.sql）
-- 翻译说明：
--   - BIGSERIAL PRIMARY KEY → BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY；
--     TIMESTAMPTZ → DATETIME(3)；DEFAULT NOW() → DEFAULT CURRENT_TIMESTAMP(3)；
--   - CREATE TABLE 显式追加 ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
--     COLLATE=utf8mb4_0900_ai_ci；
--   - CREATE [UNIQUE] INDEX IF NOT EXISTS → 去掉 IF NOT EXISTS
--     （迁移执行器保证每份迁移只执行一次）；
--   - 部分索引 WHERE deleted_at IS NULL（非唯一）→ 直接去掉 WHERE 谓词
--     （普通索引，仅性能差异）；id DESC 排序键 MySQL 8 支持，保留。

-- Gemini 显式缓存（cachedContents）的网关资源绑定。
-- public_id 是网关对客户端暴露的缓存 ID；upstream_name 是上游资源名，
-- 引用缓存的请求只能路由到 account_id 对应的账号。
CREATE TABLE IF NOT EXISTS gemini_cached_contents (
    id                BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    public_id         VARCHAR(64)  NOT NULL,
    user_id           BIGINT       NOT NULL,
    api_key_id        BIGINT       NOT NULL,
    group_id          BIGINT       NOT NULL,
    account_id        BIGINT       NOT NULL,
    upstream_name     VARCHAR(512) NOT NULL,
    model             VARCHAR(255) NOT NULL,
    upstream_model    VARCHAR(512) NOT NULL,
    display_name      VARCHAR(512) NOT NULL DEFAULT '',
    total_token_count BIGINT       NOT NULL DEFAULT 0,
    expire_time       DATETIME(3)  NOT NULL,
    created_at        DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at        DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    deleted_at        DATETIME(3)  NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE UNIQUE INDEX idx_gemini_cached_contents_public_id
    ON gemini_cached_contents (public_id);

CREATE INDEX idx_gemini_cached_contents_api_key_active
    ON gemini_cached_contents (api_key_id, id DESC);

CREATE INDEX idx_gemini_cached_contents_account
    ON gemini_cached_contents (account_id);
