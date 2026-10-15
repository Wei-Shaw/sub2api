-- OpenAI 自动路由分组：入口分组只保存候选目标，实际请求按有效倍率选择可用目标。
ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS auto_route_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS auto_route_group_ids JSONB NOT NULL DEFAULT '[]'::jsonb;

COMMENT ON COLUMN groups.auto_route_enabled IS '是否将该分组作为 OpenAI 自动路由入口';
COMMENT ON COLUMN groups.auto_route_group_ids IS 'OpenAI 自动路由候选分组 ID；按请求时有效倍率动态排序';
