-- +migrate Up

-- Sora 异步任务表：记录视频/角色/图片生成任务状态
CREATE TABLE IF NOT EXISTS sora_tasks (
    id               VARCHAR(64) PRIMARY KEY,                   -- 对外 ID (video_xxx / char_xxx / img_xxx)
    account_id       BIGINT NOT NULL,                           -- 使用的账号 ID
    api_key_id       BIGINT,                                    -- 调用方 API Key ID
    upstream_task_id VARCHAR(128) NOT NULL DEFAULT '',           -- 上游任务 ID
    object_type      VARCHAR(16) NOT NULL DEFAULT 'video',      -- video / character / image
    model            VARCHAR(64) NOT NULL,                      -- 请求模型名
    prompt           TEXT NOT NULL DEFAULT '',                   -- 生成提示词
    status           VARCHAR(16) NOT NULL DEFAULT 'queued',     -- queued / in_progress / completed / failed
    progress         INT NOT NULL DEFAULT 0,                    -- 进度 0-100
    video_url        TEXT NOT NULL DEFAULT '',                   -- 原始上游下载 URL
    stored_key       TEXT NOT NULL DEFAULT '',                   -- 存储后的 key（本地相对路径或 S3 object key）
    storage_type     VARCHAR(16) NOT NULL DEFAULT '',            -- 存储类型：local / s3 / gdrive / 空=未存储
    share_id         VARCHAR(128) NOT NULL DEFAULT '',           -- 可分享 ID
    character_info   JSONB,                                     -- 角色信息 {username, display_name}
    error_message    TEXT NOT NULL DEFAULT '',                   -- 错误信息
    error_type       VARCHAR(64) NOT NULL DEFAULT '',            -- 错误类型
    request_body     JSONB,                                     -- 原始请求体（用于上游重放）
    seconds          VARCHAR(8) NOT NULL DEFAULT '',             -- 视频时长
    size             VARCHAR(16) NOT NULL DEFAULT '',            -- 分辨率 (如 1920x1080)
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at     TIMESTAMPTZ
);

-- 仅索引未完成任务，供后台 worker 轮询
CREATE INDEX IF NOT EXISTS idx_sora_tasks_pending ON sora_tasks(status) WHERE status IN ('queued', 'in_progress');

-- 按 API Key 查询
CREATE INDEX IF NOT EXISTS idx_sora_tasks_api_key ON sora_tasks(api_key_id) WHERE api_key_id IS NOT NULL;

-- +migrate Down

DROP TABLE IF EXISTS sora_tasks;
