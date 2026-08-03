-- cyber_policy 硬阻断命中时留存的完整原始请求体。
-- 与 content_moderation_logs 一对一，随其级联删除（不设独立保留期）。
-- 请求体按需求原样存储（不截断、不脱敏），仅管理员可读。
CREATE TABLE IF NOT EXISTS cyber_policy_request_payloads (
    id                BIGSERIAL PRIMARY KEY,
    moderation_log_id BIGINT NOT NULL UNIQUE
                      REFERENCES content_moderation_logs(id) ON DELETE CASCADE,
    request_id        VARCHAR(128) NOT NULL DEFAULT '',
    user_id           BIGINT REFERENCES users(id) ON DELETE SET NULL,
    user_email        VARCHAR(255) NOT NULL DEFAULT '',
    api_key_id        BIGINT REFERENCES api_keys(id) ON DELETE SET NULL,
    group_id          BIGINT REFERENCES groups(id) ON DELETE SET NULL,
    endpoint          VARCHAR(128) NOT NULL DEFAULT '',
    protocol          VARCHAR(64) NOT NULL DEFAULT '',
    model             VARCHAR(255) NOT NULL DEFAULT '',
    request_body      TEXT NOT NULL DEFAULT '',
    body_bytes        INT NOT NULL DEFAULT 0,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
