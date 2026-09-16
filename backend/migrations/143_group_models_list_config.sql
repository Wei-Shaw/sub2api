-- 分组级自定义 /v1/models 展示列表配置。
-- 仅用于控制 GET /v1/models 的展示结果，不参与账号白名单、模型映射或网关调度。

ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS models_list_config JSONB NOT NULL DEFAULT '{}'::jsonb;
