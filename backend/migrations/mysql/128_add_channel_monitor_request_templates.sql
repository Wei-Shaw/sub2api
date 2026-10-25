-- MySQL 方言版本（翻译自 ../128_add_channel_monitor_request_templates.sql）
-- 翻译说明：
--   - BIGSERIAL → BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY；JSONB → JSON，
--     DEFAULT '{}'::jsonb → DEFAULT ('{}')（8.0.13+ 表达式默认值）。
--   - TIMESTAMPTZ → DATETIME(3)，DEFAULT NOW() → DEFAULT CURRENT_TIMESTAMP(3)。
--   - DO 块（约束存在性检查 + ADD CONSTRAINT）：迁移执行器保证单次执行，存在性分支
--     去掉后直接执行（body_mode_check / template_id_fkey 均为本迁移首次引入，无重放冲突）。
--   - 末尾部分索引 WHERE template_id IS NOT NULL 为非唯一部分索引 → 去掉 WHERE（普通索引）。

-- Migration: 128_add_channel_monitor_request_templates
-- 加请求模板表 + 给 channel_monitors 加 4 个快照字段（template_id 关联引用 + extra_headers /
-- body_override_mode / body_override 三个真正运行时使用的快照）。
--
-- 设计要点：
--  1) 模板与监控之间是「应用即拷贝」的快照语义，运行时 checker 不再回查模板表。
--     模板 UPDATE 不会自动影响监控；只有用户主动「应用到关联监控」才会刷新快照。
--  2) ON DELETE SET NULL：模板删除不级联清理监控；监控保留快照继续工作。
--  3) extra_headers / body_override 都是 JSON；body_override_mode 用 varchar（不是 enum）
--     便于将来加新模式无需 ALTER TYPE。
--  4) 同一 provider 内模板 name 唯一（允许 Anthropic + OpenAI 重名 "伪装官方客户端"）。

CREATE TABLE IF NOT EXISTS channel_monitor_request_templates (
    id            BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    name          VARCHAR(100) NOT NULL,
    provider      VARCHAR(20)  NOT NULL,
    description   VARCHAR(500) NOT NULL DEFAULT '',
    extra_headers JSON         NOT NULL DEFAULT ('{}'),
    body_override_mode VARCHAR(10) NOT NULL DEFAULT 'off',
    body_override JSON         NULL,
    created_at    DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at    DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    CONSTRAINT channel_monitor_request_templates_provider_check
        CHECK (provider IN ('openai', 'anthropic', 'gemini')),
    CONSTRAINT channel_monitor_request_templates_body_mode_check
        CHECK (body_override_mode IN ('off', 'merge', 'replace'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE UNIQUE INDEX channel_monitor_request_templates_provider_name
    ON channel_monitor_request_templates (provider, name);

-- channel_monitors 加 4 列
ALTER TABLE channel_monitors
    ADD COLUMN template_id        BIGINT      NULL;
ALTER TABLE channel_monitors
    ADD COLUMN extra_headers      JSON        NOT NULL DEFAULT ('{}');
ALTER TABLE channel_monitors
    ADD COLUMN body_override_mode VARCHAR(10) NOT NULL DEFAULT 'off';
ALTER TABLE channel_monitors
    ADD COLUMN body_override      JSON        NULL;

-- 约束 + 外键（PG 版 DO 块里的 IF NOT EXISTS 判断已按执行器单次执行语义去掉）
ALTER TABLE channel_monitors
    ADD CONSTRAINT channel_monitors_body_mode_check
    CHECK (body_override_mode IN ('off', 'merge', 'replace'));

ALTER TABLE channel_monitors
    ADD CONSTRAINT channel_monitors_template_id_fkey
    FOREIGN KEY (template_id)
    REFERENCES channel_monitor_request_templates (id)
    ON DELETE SET NULL;

CREATE INDEX idx_channel_monitors_template_id
    ON channel_monitors (template_id);
