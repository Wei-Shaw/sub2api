-- MySQL 方言版本（翻译自 ../135_content_moderation.sql）
-- 翻译说明：
--   - PG 的 ON CONFLICT (key) DO NOTHING → ON DUPLICATE KEY UPDATE id = id（settings 有 id 列）；
--     `key` 为 MySQL 保留字，加反引号。
--   - 内联 REFERENCES → 显式命名外键（MySQL 会忽略列内联 REFERENCES，必须表级约束）。
--   - JSONB → JSON，DEFAULT '{}'::jsonb → DEFAULT ('{}')（8.0.13+ 表达式默认值）；
--     TEXT NOT NULL DEFAULT '' → DEFAULT ('')（TEXT 只允许表达式默认值）。
--   - PG 版建表后的 4 条 ALTER TABLE ... ADD COLUMN IF NOT EXISTS（violation_count /
--     auto_banned / email_sent / queue_delay_ms）是历史演进的幂等补列，列已在建表语句中，
--     MySQL 无 ADD COLUMN IF NOT EXISTS，按迁移执行器单次执行语义省略（避免重复列错误）。

-- 风控中心内容审计配置与记录

INSERT INTO settings (`key`, value, updated_at)
VALUES ('risk_control_enabled', 'false', NOW(3))
ON DUPLICATE KEY UPDATE id = id;

CREATE TABLE IF NOT EXISTS content_moderation_logs (
    id                  BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    request_id          VARCHAR(128) NOT NULL DEFAULT '',
    user_id             BIGINT NULL,
    user_email          VARCHAR(255) NOT NULL DEFAULT '',
    api_key_id          BIGINT NULL,
    api_key_name        VARCHAR(100) NOT NULL DEFAULT '',
    group_id            BIGINT NULL,
    group_name          VARCHAR(255) NOT NULL DEFAULT '',
    endpoint            VARCHAR(128) NOT NULL DEFAULT '',
    provider            VARCHAR(64) NOT NULL DEFAULT '',
    model               VARCHAR(255) NOT NULL DEFAULT '',
    mode                VARCHAR(32) NOT NULL DEFAULT '',
    action              VARCHAR(32) NOT NULL DEFAULT '',
    flagged             BOOLEAN NOT NULL DEFAULT FALSE,
    highest_category    VARCHAR(64) NOT NULL DEFAULT '',
    highest_score       DECIMAL(8, 6) NOT NULL DEFAULT 0,
    category_scores     JSON NOT NULL DEFAULT ('{}'),
    threshold_snapshot  JSON NOT NULL DEFAULT ('{}'),
    input_excerpt       TEXT NOT NULL DEFAULT (''),
    upstream_latency_ms INT,
    error               TEXT NOT NULL DEFAULT (''),
    violation_count     INT NOT NULL DEFAULT 0,
    auto_banned         BOOLEAN NOT NULL DEFAULT FALSE,
    email_sent          BOOLEAN NOT NULL DEFAULT FALSE,
    queue_delay_ms      INT,
    created_at          DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    CONSTRAINT fk_content_moderation_logs_user_id FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE SET NULL,
    CONSTRAINT fk_content_moderation_logs_api_key_id FOREIGN KEY (api_key_id) REFERENCES api_keys(id) ON DELETE SET NULL,
    CONSTRAINT fk_content_moderation_logs_group_id FOREIGN KEY (group_id) REFERENCES `groups`(id) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE INDEX idx_content_moderation_logs_created_at ON content_moderation_logs(created_at DESC);
CREATE INDEX idx_content_moderation_logs_group_created_at ON content_moderation_logs(group_id, created_at DESC);
CREATE INDEX idx_content_moderation_logs_flagged_created_at ON content_moderation_logs(flagged, created_at DESC);
CREATE INDEX idx_content_moderation_logs_user_created_at ON content_moderation_logs(user_id, created_at DESC);
CREATE INDEX idx_content_moderation_logs_api_key_created_at ON content_moderation_logs(api_key_id, created_at DESC);
CREATE INDEX idx_content_moderation_logs_endpoint_created_at ON content_moderation_logs(endpoint, created_at DESC);
