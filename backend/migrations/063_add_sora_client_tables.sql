-- Migration: 063_add_sora_client_tables
-- Sora 客户端功能所需的数据库变更：
--   1. 新增 sora_generations 表：记录 Sora 客户端 UI 的生成历史
--   2. users 表新增存储配额字段
--   3. groups 表新增存储配额字段

-- ============================================================
-- 1. sora_generations 表（生成记录）
-- ============================================================
CREATE TABLE IF NOT EXISTS sora_generations (
    id               BIGSERIAL PRIMARY KEY,
    user_id          BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    api_key_id       BIGINT,

    -- 生成参数
    model            VARCHAR(64) NOT NULL,
    prompt           TEXT NOT NULL DEFAULT '',
    media_type       VARCHAR(16) NOT NULL DEFAULT 'video',    -- video / image

    -- 结果
    status           VARCHAR(16) NOT NULL DEFAULT 'pending',  -- pending / generating / completed / failed / cancelled
    media_url        TEXT NOT NULL DEFAULT '',
    media_urls       JSONB,                                   -- 多图时的 URL 数组
    file_size_bytes  BIGINT NOT NULL DEFAULT 0,
    storage_type     VARCHAR(16) NOT NULL DEFAULT 'none',     -- s3 / local / upstream / none
    s3_object_keys   JSONB,                                   -- S3 object key 数组

    -- 上游信息
    upstream_task_id VARCHAR(128) NOT NULL DEFAULT '',
    error_message    TEXT NOT NULL DEFAULT '',

    -- 时间
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at     TIMESTAMPTZ
);

-- 按用户+时间查询（作品库列表、历史记录）
CREATE INDEX IF NOT EXISTS idx_sora_gen_user_created
    ON sora_generations(user_id, created_at DESC);

-- 按用户+状态查询（恢复进行中任务）
CREATE INDEX IF NOT EXISTS idx_sora_gen_user_status
    ON sora_generations(user_id, status);

-- ============================================================
-- 2. users 表新增 Sora 存储配额字段
-- ============================================================
ALTER TABLE users
    ADD COLUMN IF NOT EXISTS sora_storage_quota_bytes BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS sora_storage_used_bytes  BIGINT NOT NULL DEFAULT 0;

-- ============================================================
-- 3. groups 表新增 Sora 存储配额字段
-- ============================================================
ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS sora_storage_quota_bytes BIGINT NOT NULL DEFAULT 0;
