-- async_video_tasks: fal 视频异步任务表（PR-1 视频链路，seedance 等）。
-- 与 async_media_tasks（图片）并行独立，按 (resolution × duration_seconds) 计费。
-- 字段与 ent/migrate/schema.go 的 AsyncVideoTasksColumns 严格一致，索引名与 ent 生成保持相同。

CREATE TABLE IF NOT EXISTS async_video_tasks (
    id                   BIGSERIAL PRIMARY KEY,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    internal_request_id  VARCHAR(64)  NOT NULL,
    upstream_request_id  VARCHAR(128),
    status_url           VARCHAR(512),
    response_url         VARCHAR(512),
    account_id           BIGINT,
    api_key_id           BIGINT       NOT NULL,
    user_id              BIGINT       NOT NULL,
    organization_id      BIGINT,
    payer_user_id        BIGINT,
    balance_source       VARCHAR(16),
    authz_generation     BIGINT,
    group_id             BIGINT,
    channel_id           BIGINT,
    facade               VARCHAR(16)  NOT NULL DEFAULT 'fal',
    requested_model      VARCHAR(200) NOT NULL,
    upstream_model       VARCHAR(200),
    resolution           VARCHAR(16),
    duration_seconds     INTEGER      NOT NULL DEFAULT 0,
    aspect_ratio         VARCHAR(16),
    status               VARCHAR(16)  NOT NULL DEFAULT 'pending',
    held_cost            NUMERIC(20,10) NOT NULL DEFAULT 0,
    final_cost           NUMERIC(20,10) NOT NULL DEFAULT 0,
    rate_multiplier      NUMERIC(10,4)  NOT NULL DEFAULT 1,
    unit_price_snapshot  NUMERIC(20,10) NOT NULL DEFAULT 0,
    request_payload      JSONB,
    result_payload       JSONB,
    video_urls           JSONB,
    cos_urls             JSONB,
    error_reason         VARCHAR(512),
    fail_deadline_at     TIMESTAMPTZ,
    finished_at          TIMESTAMPTZ,
    client_ip            VARCHAR(45),
    user_agent           VARCHAR(512),
    inbound_endpoint     VARCHAR(200),
    upstream_endpoint    VARCHAR(200)
);

CREATE UNIQUE INDEX IF NOT EXISTS asyncvideotask_internal_request_id
    ON async_video_tasks (internal_request_id);
CREATE INDEX IF NOT EXISTS asyncvideotask_upstream_request_id
    ON async_video_tasks (upstream_request_id);
CREATE INDEX IF NOT EXISTS asyncvideotask_user_id
    ON async_video_tasks (user_id);
CREATE INDEX IF NOT EXISTS asyncvideotask_organization_id_created_at
    ON async_video_tasks (organization_id, created_at);
CREATE INDEX IF NOT EXISTS asyncvideotask_api_key_id
    ON async_video_tasks (api_key_id);
CREATE INDEX IF NOT EXISTS asyncvideotask_account_id
    ON async_video_tasks (account_id);
CREATE INDEX IF NOT EXISTS asyncvideotask_status
    ON async_video_tasks (status);
CREATE INDEX IF NOT EXISTS asyncvideotask_status_fail_deadline_at
    ON async_video_tasks (status, fail_deadline_at);
CREATE INDEX IF NOT EXISTS asyncvideotask_created_at
    ON async_video_tasks (created_at);
