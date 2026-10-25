-- MySQL 方言版本（翻译自 ../145_deleted_api_key_audit.sql）
-- 翻译说明：
--   - PG 的 SET LOCAL lock_timeout / statement_timeout 无 MySQL 等价语句，跳过。
--   - BIGSERIAL → BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY；TIMESTAMPTZ → DATETIME(3)，
--     DEFAULT NOW() → DEFAULT CURRENT_TIMESTAMP(3)。
--   - `key` 为 MySQL 保留字，建表列与索引键均加反引号。
--   - ADD COLUMN IF NOT EXISTS → ADD COLUMN（按迁移执行器单次执行语义去掉）。

-- 已删除 API key 审计表:删除 key 时同步留存(明文 key、所有者、key 信息),
-- 供认证失败(INVALID_API_KEY)反查"这个失效 key 曾属于谁"。
-- 仅对本表上线后删除的 key 生效;此前已删的 key 原值已被 tombstone 覆盖,无法补录。

CREATE TABLE IF NOT EXISTS deleted_api_key_audits (
    id           BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    `key`        VARCHAR(128) NOT NULL,            -- 原 key 明文(复用 api_keys 策略),非唯一
    api_key_id   BIGINT NOT NULL,                  -- 原 api_keys.id
    user_id      BIGINT NOT NULL,                  -- 原所有者(不加外键,与 ops 表设计哲学一致)
    key_name     VARCHAR(100) NOT NULL DEFAULT '', -- 原 key 名称,便于展示
    deleted_at   DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    created_at   DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE INDEX deletedapikeyaudit_key     ON deleted_api_key_audits (`key`);
CREATE INDEX deletedapikeyaudit_user_id ON deleted_api_key_audits (user_id);

ALTER TABLE ops_error_logs
    ADD COLUMN attempted_key_prefix      VARCHAR(32) NULL,
    ADD COLUMN deleted_key_owner_user_id BIGINT NULL,
    ADD COLUMN deleted_key_name          VARCHAR(100) NULL;
