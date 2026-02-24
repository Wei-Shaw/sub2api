-- Migration: 046_add_sora_accounts
-- 新增 sora_accounts 扩展表，存储 Sora 账号的 OAuth 凭证
-- 与 accounts 主表形成双表结构：
--   - accounts: 统一账号管理和调度
--   - sora_accounts: Sora gateway 快速读取和资格校验
--
-- 设计说明：
--   - account_id 为主键，外键关联 accounts.id
--   - ON DELETE CASCADE 确保删除账号时自动清理扩展表
--   - access_token/refresh_token 与 accounts.credentials 保持同步

CREATE TABLE IF NOT EXISTS sora_accounts (
    account_id BIGINT PRIMARY KEY,
    access_token TEXT NOT NULL,
    refresh_token TEXT NOT NULL,
    session_token TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_sora_accounts_account_id
        FOREIGN KEY (account_id) REFERENCES accounts(id)
        ON DELETE CASCADE
);

-- 索引说明：主键已自动创建唯一索引，无需额外创建 idx_sora_accounts_account_id
