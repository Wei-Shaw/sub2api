-- 232_group_smart_routing.sql
-- 智能路由分组：platform=smart_routing 的分组不绑定账号，
-- 通过 smart_routing_members(JSONB) 配置成员分组及其优先级/权重。
-- 请求按优先级层调度（数值越小越先），失败后按优先级降序重试；
-- 同优先级层内按权重加权随机分流。

ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS smart_routing_members jsonb DEFAULT '[]'::jsonb;

-- 仅智能路由分组会按成员配置调度，普通分组该列恒为默认空数组。
CREATE INDEX IF NOT EXISTS idx_groups_platform_smart_routing
    ON groups (platform)
    WHERE platform = 'smart_routing' AND deleted_at IS NULL;
