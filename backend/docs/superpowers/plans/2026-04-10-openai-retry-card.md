# OpenAI Retry Card Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在 admin ops 面板新增一张独立的 OpenAI 服务端重试分布卡片，展示 active/exhausted 两组的“发生过重试的请求数”和“服务端重试总次数”，并给出 overall total。

**Architecture:** 复用现有 `/admin/ops/dashboard/openai-routing` 统计链，不新增接口，只扩展 response schema 和 repo SQL 聚合，再新增一个前端卡片组件挂到 `OpsDashboard.vue`。这样既保留现有 routing/token 卡片语义，又能最小成本补齐 retry 审计视图。

**Tech Stack:** Go, Gin, PostgreSQL, Vue 3, TypeScript, Vitest

---

### Task 1: 扩展后端统计返回结构

**Files:**
- Modify: `backend/internal/service/ops_openai_routing_stats.go`

- [ ] **Step 1: 扩展响应结构字段**

```go
type OpsOpenAIRoutingStatsResponse struct {
	TimeRange string    `json:"time_range"`
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`

	Platform string `json:"platform,omitempty"`
	GroupID  *int64 `json:"group_id,omitempty"`

	RequestCountByGroup        map[string]int64 `json:"request_count_by_group"`
	TotalTokensByGroup         map[string]int64 `json:"total_tokens_by_group"`
	InputTokensByGroup         map[string]int64 `json:"input_tokens_by_group"`
	OutputTokensByGroup        map[string]int64 `json:"output_tokens_by_group"`
	RetriedRequestCountByGroup map[string]int64 `json:"retried_request_count_by_group"`
	RetryCountByGroup          map[string]int64 `json:"retry_count_by_group"`
}
```

- [ ] **Step 2: 初始化新 map 字段**

```go
func NewOpsOpenAIRoutingStatsResponse() *OpsOpenAIRoutingStatsResponse {
	resp := &OpsOpenAIRoutingStatsResponse{
		RequestCountByGroup:        make(map[string]int64, 2),
		TotalTokensByGroup:         make(map[string]int64, 2),
		InputTokensByGroup:         make(map[string]int64, 2),
		OutputTokensByGroup:        make(map[string]int64, 2),
		RetriedRequestCountByGroup: make(map[string]int64, 2),
		RetryCountByGroup:          make(map[string]int64, 2),
	}
	for _, group := range GetOpsOpenAIRoutingStatsGroups() {
		resp.RequestCountByGroup[group] = 0
		resp.TotalTokensByGroup[group] = 0
		resp.InputTokensByGroup[group] = 0
		resp.OutputTokensByGroup[group] = 0
		resp.RetriedRequestCountByGroup[group] = 0
		resp.RetryCountByGroup[group] = 0
	}
	return resp
}
```

- [ ] **Step 3: 运行编译验证**

Run: `go test ./internal/service -run "GetOpenAIRoutingStats|OpsOpenAIRoutingStats" -count=1`

Expected: 编译通过；如无命中测试，至少 package 级通过。

### Task 2: 扩展 repo SQL 聚合

**Files:**
- Modify: `backend/internal/repository/ops_repo_openai_routing_stats.go`

- [ ] **Step 1: 在 SQL 中加入 retry 聚合列**

```sql
SELECT
  LOWER(ul.routing_target_group) AS routing_target_group,
  COUNT(*)::bigint AS request_count,
  COALESCE(SUM(COALESCE(ul.input_tokens, 0) + COALESCE(ul.output_tokens, 0)), 0)::bigint AS total_tokens,
  COALESCE(SUM(COALESCE(ul.input_tokens, 0)), 0)::bigint AS input_tokens,
  COALESCE(SUM(COALESCE(ul.output_tokens, 0)), 0)::bigint AS output_tokens,
  COUNT(*) FILTER (WHERE COALESCE(ul.routing_failover_count, 0) > 0)::bigint AS retried_request_count,
  COALESCE(SUM(COALESCE(ul.routing_failover_count, 0)), 0)::bigint AS retry_count
FROM usage_logs ul
...
```

- [ ] **Step 2: 扩展 rows.Scan 与 response 写回**

```go
var retriedRequestCount int64
var retryCount int64
if err := rows.Scan(&targetGroup, &requestCount, &totalTokens, &inputTokens, &outputTokens, &retriedRequestCount, &retryCount); err != nil {
	return nil, err
}
resp.RetriedRequestCountByGroup[targetGroup] = retriedRequestCount
resp.RetryCountByGroup[targetGroup] = retryCount
```

- [ ] **Step 3: 运行 repo/service 验证**

Run: `go test ./internal/repository ./internal/service -run "OpenAIRoutingStats" -count=1`

Expected: 统计链测试/编译通过。

### Task 3: 前端 API 类型同步

**Files:**
- Modify: `frontend/src/api/admin/ops.ts`

- [ ] **Step 1: 扩展响应类型**

```ts
export interface OpsOpenAIRoutingStatsResponse {
  time_range: string
  start_time: string
  end_time: string
  platform?: string
  group_id?: number | null
  request_count_by_group: Record<string, number>
  total_tokens_by_group: Record<string, number>
  input_tokens_by_group: Record<string, number>
  output_tokens_by_group: Record<string, number>
  retried_request_count_by_group: Record<string, number>
  retry_count_by_group: Record<string, number>
}
```

- [ ] **Step 2: 运行前端类型检查**

Run: `pnpm typecheck`

Expected: 通过，若新卡片未实现则只验证类型扩展不破坏现有代码。

### Task 4: 新增 Retry 卡片组件

**Files:**
- Create: `frontend/src/views/admin/ops/components/OpsOpenAIRetryCard.vue`
- Modify: `frontend/src/views/admin/ops/OpsDashboard.vue`

- [ ] **Step 1: 创建新卡片组件骨架**

核心结构参考 `OpsOpenAIRoutingCard.vue`，但指标改为：

```ts
const metrics = computed(() => {
  const data = response.value
  const retriedRequestCountByGroup = data?.retried_request_count_by_group ?? {}
  const retryCountByGroup = data?.retry_count_by_group ?? {}

  return [
    {
      key: 'retried_request_count',
      label: t('admin.ops.openaiRetry.retriedRequestCount'),
      active: retriedRequestCountByGroup.active ?? 0,
      exhausted: retriedRequestCountByGroup.exhausted ?? 0,
    },
    {
      key: 'retry_count',
      label: t('admin.ops.openaiRetry.retryCount'),
      active: retryCountByGroup.active ?? 0,
      exhausted: retryCountByGroup.exhausted ?? 0,
    },
  ]
})
```

- [ ] **Step 2: 为每个指标展示 overall total**

```ts
function total(metric: { active: number; exhausted: number }) {
  return (metric.active ?? 0) + (metric.exhausted ?? 0)
}
```

模板中新增一行 total，例如：

```vue
<div class="mt-3 border-t border-gray-200 pt-3 text-sm dark:border-dark-700">
  <div class="flex items-center justify-between gap-3">
    <span class="font-medium text-gray-600 dark:text-gray-300">{{ t('common.total') }}</span>
    <span class="text-base font-semibold text-gray-900 dark:text-white">{{ formatMetricValue(total(metric)) }}</span>
  </div>
</div>
```

- [ ] **Step 3: 在 `OpsDashboard.vue` 中挂载新卡片**

```vue
<OpsOpenAIRoutingCard ... />
<OpsOpenAIRetryCard
  :platform-filter="platformFilter"
  :group-id-filter="groupIdFilter"
  :time-range="timeRange"
  :start-time="startTime"
  :end-time="endTime"
  :refresh-token="refreshToken"
/>
```

- [ ] **Step 4: 运行前端类型检查**

Run: `pnpm typecheck`

Expected: 通过。

### Task 5: 文案与基础验证

**Files:**
- Modify: `frontend/src/i18n/locales/en.ts`
- Modify: `frontend/src/i18n/locales/zh.ts`

- [ ] **Step 1: 新增 i18n 文案**

```ts
openaiRetry: {
  title: 'OpenAI Server Retry Distribution',
  subtitle: 'Retry distribution of persisted OpenAI requests grouped by target group',
  retriedRequestCount: 'Requests With Retry',
  retryCount: 'Total Retry Count',
  empty: 'No retry data under current filters',
  failedToLoad: 'Failed to load retry stats',
}
```

```ts
openaiRetry: {
  title: 'OpenAI 服务端重试分布',
  subtitle: '按目标组统计已落库 OpenAI 请求中的服务端重试分布',
  retriedRequestCount: '发生过重试的请求数',
  retryCount: '服务端重试总次数',
  empty: '当前筛选条件下暂无重试数据',
  failedToLoad: '重试统计加载失败',
}
```

- [ ] **Step 2: 运行最终验证**

Run:
- `pnpm typecheck`
- `go test ./internal/handler ./internal/repository ./internal/server/... -count=1`
- `go test -tags unit ./internal/service -count=1`

Expected: 前后端验证全部通过。
