-- Gemini 显式缓存（cachedContents）的网关资源绑定。
-- public_id 是网关对客户端暴露的缓存 ID；upstream_name 是上游资源名，
-- 引用缓存的请求只能路由到 account_id 对应的账号。
CREATE TABLE IF NOT EXISTS gemini_cached_contents (
    id                BIGSERIAL PRIMARY KEY,
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
    expire_time       TIMESTAMPTZ  NOT NULL,
    created_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    deleted_at        TIMESTAMPTZ  NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_gemini_cached_contents_public_id
    ON gemini_cached_contents (public_id);

CREATE INDEX IF NOT EXISTS idx_gemini_cached_contents_api_key_active
    ON gemini_cached_contents (api_key_id, id DESC)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_gemini_cached_contents_account
    ON gemini_cached_contents (account_id);
