-- Sub2API 初始化数据库迁移脚本
-- PostgreSQL 15+

-- 1. proxies 代理IP表（无外键依赖）
CREATE TABLE IF NOT EXISTS proxies (
    id              BIGSERIAL PRIMARY KEY,
    name            VARCHAR(100) NOT NULL,
    protocol        VARCHAR(20) NOT NULL,                 -- http/https/socks5
    host            VARCHAR(255) NOT NULL,
    port            INT NOT NULL,
    username        VARCHAR(100),
    password        VARCHAR(100),
    status          VARCHAR(20) NOT NULL DEFAULT 'active', -- active/disabled
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at      TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_proxies_status ON proxies(status);
CREATE INDEX IF NOT EXISTS idx_proxies_deleted_at ON proxies(deleted_at);

-- 2. groups 分组表（无外键依赖）
CREATE TABLE IF NOT EXISTS groups (
    id              BIGSERIAL PRIMARY KEY,
    name            VARCHAR(100) NOT NULL UNIQUE,
    description     TEXT,
    rate_multiplier DECIMAL(10, 4) NOT NULL DEFAULT 1.0,  -- 费率倍率
    is_exclusive    BOOLEAN NOT NULL DEFAULT FALSE,       -- 是否专属分组
    status          VARCHAR(20) NOT NULL DEFAULT 'active', -- active/disabled
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at      TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_groups_name ON groups(name);
CREATE INDEX IF NOT EXISTS idx_groups_status ON groups(status);
CREATE INDEX IF NOT EXISTS idx_groups_is_exclusive ON groups(is_exclusive);
CREATE INDEX IF NOT EXISTS idx_groups_deleted_at ON groups(deleted_at);

-- 3. users 用户表（无外键依赖）
CREATE TABLE IF NOT EXISTS users (
    id              BIGSERIAL PRIMARY KEY,
    email           VARCHAR(255) NOT NULL UNIQUE,
    password_hash   VARCHAR(255) NOT NULL,
    role            VARCHAR(20) NOT NULL DEFAULT 'user',  -- admin/user
    balance         DECIMAL(20, 8) NOT NULL DEFAULT 0,    -- 余额（可为负数）
    concurrency     INT NOT NULL DEFAULT 5,               -- 并发数限制
    status          VARCHAR(20) NOT NULL DEFAULT 'active', -- active/disabled
    allowed_groups  BIGINT[] DEFAULT NULL,                -- 允许绑定的分组ID列表
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at      TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);
CREATE INDEX IF NOT EXISTS idx_users_status ON users(status);
CREATE INDEX IF NOT EXISTS idx_users_deleted_at ON users(deleted_at);

-- 4. accounts 上游账号表（依赖proxies）
CREATE TABLE IF NOT EXISTS accounts (
    id              BIGSERIAL PRIMARY KEY,
    name            VARCHAR(100) NOT NULL,
    platform        VARCHAR(50) NOT NULL,                 -- anthropic/openai/gemini
    type            VARCHAR(20) NOT NULL,                 -- oauth/apikey
    credentials     JSONB NOT NULL DEFAULT '{}',          -- 凭证信息（加密存储）
    extra           JSONB NOT NULL DEFAULT '{}',          -- 扩展信息
    proxy_id        BIGINT REFERENCES proxies(id) ON DELETE SET NULL,
    concurrency     INT NOT NULL DEFAULT 3,               -- 账号并发限制
    priority        INT NOT NULL DEFAULT 50,              -- 调度优先级(1-100，越小越高)
    status          VARCHAR(20) NOT NULL DEFAULT 'active', -- active/disabled/error
    error_message   TEXT,
    last_used_at    TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at      TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_accounts_platform ON accounts(platform);
CREATE INDEX IF NOT EXISTS idx_accounts_type ON accounts(type);
CREATE INDEX IF NOT EXISTS idx_accounts_status ON accounts(status);
CREATE INDEX IF NOT EXISTS idx_accounts_proxy_id ON accounts(proxy_id);
CREATE INDEX IF NOT EXISTS idx_accounts_priority ON accounts(priority);
CREATE INDEX IF NOT EXISTS idx_accounts_last_used_at ON accounts(last_used_at);
CREATE INDEX IF NOT EXISTS idx_accounts_deleted_at ON accounts(deleted_at);

-- 5. api_keys API密钥表（依赖users, groups）
CREATE TABLE IF NOT EXISTS api_keys (
    id              BIGSERIAL PRIMARY KEY,
    user_id         BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    key             VARCHAR(64) NOT NULL UNIQUE,          -- sk-xxx格式
    name            VARCHAR(100) NOT NULL,
    group_id        BIGINT REFERENCES groups(id) ON DELETE SET NULL,
    status          VARCHAR(20) NOT NULL DEFAULT 'active', -- active/disabled
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at      TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_api_keys_key ON api_keys(key);
CREATE INDEX IF NOT EXISTS idx_api_keys_user_id ON api_keys(user_id);
CREATE INDEX IF NOT EXISTS idx_api_keys_group_id ON api_keys(group_id);
CREATE INDEX IF NOT EXISTS idx_api_keys_status ON api_keys(status);
CREATE INDEX IF NOT EXISTS idx_api_keys_deleted_at ON api_keys(deleted_at);

-- 6. account_groups 账号-分组关联表（依赖accounts, groups）
CREATE TABLE IF NOT EXISTS account_groups (
    account_id      BIGINT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    group_id        BIGINT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    priority        INT NOT NULL DEFAULT 50,              -- 分组内优先级
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (account_id, group_id)
);

CREATE INDEX IF NOT EXISTS idx_account_groups_group_id ON account_groups(group_id);
CREATE INDEX IF NOT EXISTS idx_account_groups_priority ON account_groups(priority);

-- 7. redeem_codes 卡密表（依赖users）
CREATE TABLE IF NOT EXISTS redeem_codes (
    id              BIGSERIAL PRIMARY KEY,
    code            VARCHAR(32) NOT NULL UNIQUE,          -- 兑换码
    type            VARCHAR(20) NOT NULL DEFAULT 'balance', -- balance
    value           DECIMAL(20, 8) NOT NULL,              -- 面值（USD）
    status          VARCHAR(20) NOT NULL DEFAULT 'unused', -- unused/used
    used_by         BIGINT REFERENCES users(id) ON DELETE SET NULL,
    used_at         TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_redeem_codes_code ON redeem_codes(code);
CREATE INDEX IF NOT EXISTS idx_redeem_codes_status ON redeem_codes(status);
CREATE INDEX IF NOT EXISTS idx_redeem_codes_used_by ON redeem_codes(used_by);

-- 8. usage_logs 使用记录表（依赖users, api_keys, accounts）
CREATE TABLE IF NOT EXISTS usage_logs (
    id                          BIGSERIAL PRIMARY KEY,
    user_id                     BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    api_key_id                  BIGINT NOT NULL REFERENCES api_keys(id) ON DELETE CASCADE,
    account_id                  BIGINT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    request_id                  VARCHAR(64),
    model                       VARCHAR(100) NOT NULL,

    -- Token使用量（4类）
    input_tokens                INT NOT NULL DEFAULT 0,
    output_tokens               INT NOT NULL DEFAULT 0,
    cache_creation_tokens       INT NOT NULL DEFAULT 0,
    cache_read_tokens           INT NOT NULL DEFAULT 0,

    -- 详细的缓存创建分类
    cache_creation_5m_tokens    INT NOT NULL DEFAULT 0,
    cache_creation_1h_tokens    INT NOT NULL DEFAULT 0,

    -- 费用（USD）
    input_cost                  DECIMAL(20, 10) NOT NULL DEFAULT 0,
    output_cost                 DECIMAL(20, 10) NOT NULL DEFAULT 0,
    cache_creation_cost         DECIMAL(20, 10) NOT NULL DEFAULT 0,
    cache_read_cost             DECIMAL(20, 10) NOT NULL DEFAULT 0,
    total_cost                  DECIMAL(20, 10) NOT NULL DEFAULT 0,  -- 原始总费用
    actual_cost                 DECIMAL(20, 10) NOT NULL DEFAULT 0,  -- 实际扣除费用

    -- 元数据
    stream                      BOOLEAN NOT NULL DEFAULT FALSE,
    duration_ms                 INT,

    created_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_usage_logs_user_id ON usage_logs(user_id);
CREATE INDEX IF NOT EXISTS idx_usage_logs_api_key_id ON usage_logs(api_key_id);
CREATE INDEX IF NOT EXISTS idx_usage_logs_account_id ON usage_logs(account_id);
CREATE INDEX IF NOT EXISTS idx_usage_logs_model ON usage_logs(model);
CREATE INDEX IF NOT EXISTS idx_usage_logs_created_at ON usage_logs(created_at);
CREATE INDEX IF NOT EXISTS idx_usage_logs_user_created ON usage_logs(user_id, created_at);
