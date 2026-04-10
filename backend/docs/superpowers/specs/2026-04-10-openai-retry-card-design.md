# OpenAI 服务端重试运维卡片设计

## 背景

当前运维面板已有 `OpsOpenAIRoutingCard`，用于展示 OpenAI `routing_target_group=active/exhausted` 的请求量与 token 体量。

但服务端重试相关数据虽然已经写入 `usage_logs`（如 `routing_failover_count`、`routing_failover_final_reason`），只存在于请求详情和 usage 表格中，没有一张独立的运维聚合卡片，导致无法快速判断：

- 某个目标组承接的请求里，有多少发生过服务端重试
- 某个目标组累计发生了多少次服务端重试

## 目标

新增一张独立的 `OpsOpenAIRetryCard`，在不改变现有 `OpsOpenAIRoutingCard` 语义的前提下，补充 OpenAI 服务端重试分布的聚合可视化。

## 非目标

- 不新增新的 dashboard filter
- 不做实时中的“正在重试”监控
- 不把 retry 指标混进现有 routing/token 卡片
- 不新增新的 usage log 字段

## 数据口径

数据源继续使用 `usage_logs`，并只统计：

- `routing_target_group in ('active', 'exhausted')`

新增两组聚合：

1. `retried_request_count_by_group`
   - 含义：`routing_failover_count > 0` 的请求数
   - 口径：`count(*) filter (where routing_failover_count > 0)`

2. `retry_count_by_group`
   - 含义：服务端重试总次数
   - 口径：`sum(coalesce(routing_failover_count, 0))`

前端不要求后端单独返回 total，而是本地计算：

- `overall = active + exhausted`

说明：这张卡片统计的是**已成功落库请求**上的服务端重试数据，因此它属于审计/分布统计，不等于实时中的全部重试尝试。

## 后端设计

沿用现有接口链，不新开 endpoint：

- `frontend/src/api/admin/ops.ts`
- `backend/internal/handler/admin/ops_dashboard_handler.go`
- `backend/internal/service/ops_openai_routing_stats.go`
- `backend/internal/repository/ops_repo_openai_routing_stats.go`

在 `OpsOpenAIRoutingStatsResponse` 中新增：

- `retried_request_count_by_group map[string]int64`
- `retry_count_by_group map[string]int64`

在 repo SQL 中新增两列聚合：

- `COUNT(*) FILTER (WHERE COALESCE(ul.routing_failover_count, 0) > 0)`
- `SUM(COALESCE(ul.routing_failover_count, 0))`

并写回到上述两个 map 中，保持现有 `active/exhausted` 初始化策略不变。

## 前端设计

新增独立卡片组件：

- `frontend/src/views/admin/ops/components/OpsOpenAIRetryCard.vue`

挂载位置：

- `OpsDashboard.vue` 中，紧邻现有 `OpsOpenAIRoutingCard`

展示形式：

- 保持与 `OpsOpenAIRoutingCard` 一致的卡片视觉风格
- 每个指标一张子卡
- 每张子卡展示：
  - `active`
  - `exhausted`
  - `overall total`

新卡片只展示两个指标：

- 发生过重试的请求数
- 服务端重试总次数

空态与错误态复用现有模式：

- 接口失败：显示错误提示条
- 两组指标都为 0：显示 `暂无数据`

## 测试与验证

后端：

- repo/service 层补聚合测试
- 现有 handler/server 测试确保接口仍可访问

前端：

- 新卡片组件测试：
  - 正常展示 active/exhausted/overall
  - 空态
  - 错误态
- `pnpm typecheck`

## 风险与约束

- 该卡片只反映“已写库请求”的 retry 分布，不能替代实时重试监控。
- 如果后续要做实时 retry 观测，应单独走日志/指标链，不应继续复用这张分布卡片。

## 推荐实现顺序

1. 扩展 `OpsOpenAIRoutingStatsResponse` 和 repo 聚合 SQL
2. 打通 handler/service 返回
3. 新增 `OpsOpenAIRetryCard.vue`
4. 将新卡片挂到 `OpsDashboard.vue`
5. 补测试并验证
