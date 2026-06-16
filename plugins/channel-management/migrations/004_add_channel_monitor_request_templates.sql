-- Migration: 128_add_channel_monitor_request_templates
-- 加请求模板表 + 给 channel_monitors 加 4 个快照字段（template_id 关联引用 + extra_headers /
-- body_override_mode / body_override 三个真正运行时使用的快照）。
--
-- 设计要点：
--  1) 模板与监控之间是「应用即拷贝」的快照语义，运行时 checker 不再回查模板表。
--     模板 UPDATE 不会自动影响监控；只有用户主动「应用到关联监控」才会刷新快照。
--  2) ON DELETE SET NULL：模板删除不级联清理监控；监控保留快照继续工作。
--  3) extra_headers / body_override 都是 JSONB；body_override_mode 用 varchar（不是 enum）
--     便于将来加新模式无需 ALTER TYPE。
--  4) 同一 provider 内模板 name 唯一（允许 Anthropic + OpenAI 重名 "伪装官方客户端"）。

CREATE TABLE IF NOT EXISTS channel_monitor_request_templates (
    id            BIGSERIAL    PRIMARY KEY,
    name          VARCHAR(100) NOT NULL,
    provider      VARCHAR(20)  NOT NULL,
    description   VARCHAR(500) NOT NULL DEFAULT '',
    extra_headers JSONB        NOT NULL DEFAULT '{}'::jsonb,
    body_override_mode VARCHAR(10) NOT NULL DEFAULT 'off',
    body_override JSONB        NULL,
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT channel_monitor_request_templates_provider_check
        CHECK (provider IN ('openai', 'anthropic', 'gemini')),
    CONSTRAINT channel_monitor_request_templates_body_mode_check
        CHECK (body_override_mode IN ('off', 'merge', 'replace'))
);

CREATE UNIQUE INDEX IF NOT EXISTS channel_monitor_request_templates_provider_name
    ON channel_monitor_request_templates (provider, name);

-- channel_monitors 加 4 列（ADD COLUMN IF NOT EXISTS 需要 PG 9.6+，生产使用 PG 16）
ALTER TABLE channel_monitors
    ADD COLUMN IF NOT EXISTS template_id        BIGINT      NULL;
ALTER TABLE channel_monitors
    ADD COLUMN IF NOT EXISTS extra_headers      JSONB       NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE channel_monitors
    ADD COLUMN IF NOT EXISTS body_override_mode VARCHAR(10) NOT NULL DEFAULT 'off';
ALTER TABLE channel_monitors
    ADD COLUMN IF NOT EXISTS body_override      JSONB       NULL;

-- 约束 + 外键（DO 块里 IF NOT EXISTS 判断，保证幂等）
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.table_constraints
        WHERE constraint_name = 'channel_monitors_body_mode_check'
          AND table_name = 'channel_monitors'
    ) THEN
        ALTER TABLE channel_monitors
            ADD CONSTRAINT channel_monitors_body_mode_check
            CHECK (body_override_mode IN ('off', 'merge', 'replace'));
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM information_schema.table_constraints
        WHERE constraint_name = 'channel_monitors_template_id_fkey'
          AND table_name = 'channel_monitors'
    ) THEN
        ALTER TABLE channel_monitors
            ADD CONSTRAINT channel_monitors_template_id_fkey
            FOREIGN KEY (template_id)
            REFERENCES channel_monitor_request_templates (id)
            ON DELETE SET NULL;
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_channel_monitors_template_id
    ON channel_monitors (template_id)
    WHERE template_id IS NOT NULL;
