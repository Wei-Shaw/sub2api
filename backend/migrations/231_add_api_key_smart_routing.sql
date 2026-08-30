-- API Key 智能路由：启用后不再绑定单一分组，每次请求按请求的 model 自动选组。
-- smart_routing_config 为 JSONB，结构见 domain.SmartRoutingConfig：
--   { "exclude_group_ids": [...], "priorities": {"<gid>": n}, "weights": {"<gid>": n} }
ALTER TABLE api_keys
    ADD COLUMN IF NOT EXISTS smart_routing_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS smart_routing_config JSONB;
