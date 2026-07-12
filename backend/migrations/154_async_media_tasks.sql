-- 异步媒体任务表（fal 等异步图片平台）。
-- 与只追加的 usage_logs 不同，本表可变，承载任务完整生命周期：
-- pending → running → succeeded / failed→refunded / expired，由后台 reconciler 兜底对账。

CREATE TABLE IF NOT EXISTS async_media_tasks (
    id                   BIGSERIAL    PRIMARY KEY,

    -- 标识与上游关联
    internal_request_id  VARCHAR(64)  NOT NULL,
    upstream_request_id  VARCHAR(128),
    status_url           VARCHAR(512),
    response_url         VARCHAR(512),

    -- 归属维度
    account_id           BIGINT,
    api_key_id           BIGINT       NOT NULL,
    user_id              BIGINT       NOT NULL,
    group_id             BIGINT,
    channel_id           BIGINT,

    -- 门面与模型
    facade               VARCHAR(16)  NOT NULL DEFAULT 'openai',
    requested_model      VARCHAR(100) NOT NULL,
    upstream_model       VARCHAR(100),

    -- 图片参数
    image_size           VARCHAR(32),
    quality              VARCHAR(16),
    num_images           INT          NOT NULL DEFAULT 1,

    -- 状态与计费
    status               VARCHAR(16)  NOT NULL DEFAULT 'pending',
    held_cost            DECIMAL(20,10) NOT NULL DEFAULT 0,
    final_cost           DECIMAL(20,10) NOT NULL DEFAULT 0,
    rate_multiplier      DECIMAL(10,4)  NOT NULL DEFAULT 1,
    size_tier            VARCHAR(16),

    -- 结果与转存
    image_urls           JSONB,
    cos_urls             JSONB,

    -- 错误与超时
    error_reason         VARCHAR(512),
    fail_deadline_at     TIMESTAMPTZ,
    finished_at          TIMESTAMPTZ,

    created_at           TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

-- 内部请求 ID 唯一（对外暴露给客户端轮询）
CREATE UNIQUE INDEX IF NOT EXISTS asyncmediatask_internal_request_id_uq
    ON async_media_tasks (internal_request_id);

CREATE INDEX IF NOT EXISTS asyncmediatask_upstream_request_id
    ON async_media_tasks (upstream_request_id);
CREATE INDEX IF NOT EXISTS asyncmediatask_user_id
    ON async_media_tasks (user_id);
CREATE INDEX IF NOT EXISTS asyncmediatask_api_key_id
    ON async_media_tasks (api_key_id);
CREATE INDEX IF NOT EXISTS asyncmediatask_account_id
    ON async_media_tasks (account_id);
CREATE INDEX IF NOT EXISTS asyncmediatask_status
    ON async_media_tasks (status);
-- reconciler 扫描未终结任务：status + fail_deadline_at
CREATE INDEX IF NOT EXISTS asyncmediatask_status_fail_deadline_at
    ON async_media_tasks (status, fail_deadline_at);
CREATE INDEX IF NOT EXISTS asyncmediatask_created_at
    ON async_media_tasks (created_at);
