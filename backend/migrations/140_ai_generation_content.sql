-- M1 采集口：AI 生产内容采集（prompt + response），与 usage_logs 旁路、1:1 关联。
-- 设计见 _review/M1A_collector_20260618/。内容入库前已脱敏；表与计费热表 usage_logs 解耦，
-- 便于内容/PII 独立保留期与清理。response_* 列在 M1-B.1 预留，由 M1-B.2 的限容 tee 填充。

CREATE TABLE IF NOT EXISTS ai_generation_content (
    id                   BIGSERIAL PRIMARY KEY,
    -- 归因（与 usage_logs 以 (api_key_id, request_id) 1:1 关联）
    request_id           VARCHAR(128) NOT NULL DEFAULT '',
    api_key_id           BIGINT REFERENCES api_keys(id) ON DELETE SET NULL,
    user_id              BIGINT REFERENCES users(id) ON DELETE SET NULL,
    group_id             BIGINT REFERENCES groups(id) ON DELETE SET NULL,
    account_id           BIGINT REFERENCES accounts(id) ON DELETE SET NULL,
    model                VARCHAR(255) NOT NULL DEFAULT '',
    request_payload_hash VARCHAR(128) NOT NULL DEFAULT '',
    -- 采集内容（入库前已脱敏）
    prompt_redacted      TEXT NOT NULL DEFAULT '',
    response_redacted    TEXT NOT NULL DEFAULT '',
    prompt_bytes         INT NOT NULL DEFAULT 0,   -- 原始 prompt 字节数（脱敏/截断前）
    response_bytes       INT NOT NULL DEFAULT 0,   -- 客户端实收响应总字节（M1-B.2 填充）
    response_truncated   BOOLEAN NOT NULL DEFAULT FALSE,
    redaction_version    INT NOT NULL DEFAULT 1,   -- 脱敏规则版本（规则变更即 bump）
    -- 预留：质量/采用信号（M1 不写，留给 M5/QCanvas 跨仓回流）
    task_id              BIGINT,
    adoption_status      VARCHAR(32) NOT NULL DEFAULT '',
    quality_score        DECIMAL(8, 6),
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 与 usage_logs 的 request 去重哲学一致：同 (api_key_id, request_id) 只采一条；空 request_id 不参与去重。
CREATE UNIQUE INDEX IF NOT EXISTS uq_ai_generation_content_apikey_request
    ON ai_generation_content(api_key_id, request_id)
    WHERE request_id <> '';
CREATE INDEX IF NOT EXISTS idx_ai_generation_content_created_at
    ON ai_generation_content(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_ai_generation_content_user_created_at
    ON ai_generation_content(user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_ai_generation_content_group_created_at
    ON ai_generation_content(group_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_ai_generation_content_task_id
    ON ai_generation_content(task_id)
    WHERE task_id IS NOT NULL;
