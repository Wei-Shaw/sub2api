-- MySQL 方言版本（翻译自 ../143_group_models_list_config.sql）
-- 翻译说明：`groups` 为 MySQL 8 保留字加反引号；JSONB NOT NULL DEFAULT '{}'::jsonb
-- → JSON NOT NULL DEFAULT ('{}')（8.0.13+ 表达式默认值）。

-- 分组级自定义 /v1/models 展示列表配置。
-- 仅用于控制 GET /v1/models 的展示结果，不参与账号白名单、模型映射或网关调度。

ALTER TABLE `groups`
    ADD COLUMN models_list_config JSON NOT NULL DEFAULT ('{}');
