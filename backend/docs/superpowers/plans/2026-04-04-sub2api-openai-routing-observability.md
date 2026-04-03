# OpenAI 路由观测 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在使用记录和运维监控中稳定展示 OpenAI 路由快照，并按 active/exhausted 目标组输出请求次数、总 tokens、输入/输出 tokens 分布。

**Architecture:** 在 OpenAI 请求链路中生成一份 request-scoped `routing snapshot`，并把它持久化到 `usage_logs` 与 `ops_error_logs`。查询层直接基于持久字段做 usage/request-details 展示与 ops dashboard 聚合，不从结构化日志回放，也不新建独立 attempt 事件表。

**Tech Stack:** Go, Gin, PostgreSQL migration SQL, repository/service/handler layering, Vue 3, Vitest, go test.

---

## 文件结构

- `backend/internal/service/openai_routing_observability.go`
  新建 routing snapshot 结构与辅助函数。
- `backend/internal/service/openai_routing_observability_test.go`
  新建后端单测，锁定 snapshot 构造与 failover 计数。
- `backend/internal/service/usage_log.go`
  给 `UsageLog` 增加 routing 字段。
- `backend/internal/service/ops_port.go`
  给 `OpsInsertErrorLogInput` 增加 routing 字段。
- `backend/internal/service/openai_gateway_service.go`
  成功路径写 usage 时携带 snapshot。
- `backend/internal/handler/openai_gateway_handler.go`
  在 OpenAI 入口拼装 snapshot，并把 `target_group` / `schedule_layer` / failover 摘要填进去。
- `backend/internal/handler/ops_error_logger.go`
  错误路径把 snapshot 注入 `OpsInsertErrorLogInput`。
- `backend/ent/schema/usage_log.go`
  为 `usage_logs` 增加 routing 列。
- `backend/migrations/082_add_openai_routing_observability.sql`
  为 `usage_logs` / `ops_error_logs` 增加 routing 列和必要索引。
- `backend/internal/repository/usage_log_repo.go`
  扩展 usage log insert/select/filter。
- `backend/internal/repository/ops_repo.go`
  扩展 `ops_error_logs` 写入、`request details` 读取，以及新的 routing 聚合查询。
- `backend/internal/service/ops_request_details.go`
  让 request details 返回 routing 字段。
- `backend/internal/service/ops_openai_routing_stats.go`
  新建 routing 聚合 service 模型和聚合逻辑。
- `backend/internal/handler/admin/ops_dashboard_handler.go`
  新增 OpenAI routing dashboard 接口。
- `backend/internal/server/routes/admin.go`
  注册新的 ops dashboard 路由。
- `backend/internal/handler/dto/mappers.go`
  扩展 admin usage log DTO 映射。
- `backend/internal/handler/dto/types.go`
  扩展 usage/request details DTO 类型。
- `frontend/src/types/index.ts`
  扩展 `AdminUsageLog` 的 routing 字段。
- `frontend/src/api/admin/ops.ts`
  扩展 `OpsRequestDetail` 并新增 OpenAI routing 聚合 API。
- `frontend/src/views/admin/UsageView.vue`
  增加 routing 列、筛选项和导出字段。
- `frontend/src/views/admin/ops/components/OpsRequestDetailsModal.vue`
  显示请求级 routing 明细。
- `frontend/src/views/admin/ops/components/OpsOpenAIRoutingCard.vue`
  新建 dashboard 卡片，展示 active/exhausted 次数与 token 分布。
- `frontend/src/views/admin/ops/OpsDashboard.vue`
  挂载新的 routing 卡片。

## 任务拆分

### Task 1: 定义 routing snapshot 语义并接入 OpenAI 请求链路

**Files:**
- Create: `backend/internal/service/openai_routing_observability.go`
- Create: `backend/internal/service/openai_routing_observability_test.go`
- Modify: `backend/internal/handler/openai_gateway_handler.go`
- Modify: `backend/internal/handler/openai_gateway_handler_test.go`

- [ ] **Step 1: 写失败测试，先锁定 snapshot 基础字段和 failover 计数**

```go
func TestOpenAIRoutingSnapshot_FromSelection(t *testing.T) {
	account := &Account{ID: 66, Name: "acc-66"}
	snap := NewOpenAIRoutingSnapshot(OpenAIRoutingSnapshotInput{
		TargetGroup:    TargetGroupExhausted,
		ScheduleLayer:  openAIAccountScheduleLayerLoadBalance,
		Account:        account,
		RequestedModel: "gpt-5.4-Sys",
		EffectiveModel: "gpt-5.4",
	})

	if snap.TargetGroup != "exhausted" { t.Fatalf("target group = %q", snap.TargetGroup) }
	if snap.ScheduleLayer != string(openAIAccountScheduleLayerLoadBalance) { t.Fatalf("schedule layer = %q", snap.ScheduleLayer) }
	if snap.SelectedAccountID == nil || *snap.SelectedAccountID != 66 { t.Fatalf("selected account id missing") }
}

func TestOpenAIRoutingSnapshot_RecordFailover(t *testing.T) {
	snap := NewOpenAIRoutingSnapshot(OpenAIRoutingSnapshotInput{TargetGroup: TargetGroupActive, ScheduleLayer: openAIAccountScheduleLayerStickySession, RequestedModel: "gpt-5.4", EffectiveModel: "gpt-5.4"})
	snap.RecordFailover("upstream_502")
	snap.RecordFailover("selected_exhausted_fallback")
	if snap.FailoverCount != 2 { t.Fatalf("failover count = %d", snap.FailoverCount) }
	if snap.FailoverFinalReason != "selected_exhausted_fallback" { t.Fatalf("final reason = %q", snap.FailoverFinalReason) }
}
```

- [ ] **Step 2: 运行失败测试，确认 snapshot 结构尚不存在**

Run: `go test ./internal/service -run TestOpenAIRoutingSnapshot_ -count=1`
Expected: FAIL，提示 `OpenAIRoutingSnapshot` / `NewOpenAIRoutingSnapshot` 未定义。

- [ ] **Step 3: 写最小实现，定义 snapshot 结构**

```go
type OpenAIRoutingSnapshot struct {
	TargetGroup         string
	ScheduleLayer       string
	SelectedAccountID   *int64
	SelectedAccountName *string
	RequestedModel      string
	EffectiveModel      string
	FailoverCount       int
	FailoverFinalReason string
}

type OpenAIRoutingSnapshotInput struct {
	TargetGroup    AccountTargetGroup
	ScheduleLayer  openAIAccountScheduleLayer
	Account        *Account
	RequestedModel string
	EffectiveModel string
}
```

- [ ] **Step 4: 在 OpenAI handler 里先生成 snapshot，但此任务先不做持久化**

```go
snapshot := service.NewOpenAIRoutingSnapshot(service.OpenAIRoutingSnapshotInput{
	TargetGroup:    targetGroup,
	ScheduleLayer:  scheduleDecision.Layer,
	Account:        account,
	RequestedModel: requestedModel,
	EffectiveModel: reqModel,
})
```

- [ ] **Step 5: 跑测试确认通过**

Run: `go test ./internal/service -run TestOpenAIRoutingSnapshot_ -count=1`
Expected: PASS。

- [ ] **Step 6: Commit**

```bash
git add backend/internal/service/openai_routing_observability.go backend/internal/service/openai_routing_observability_test.go backend/internal/handler/openai_gateway_handler.go backend/internal/handler/openai_gateway_handler_test.go
git commit -m "feat(openai): 定义路由观测快照语义"
```

### Task 2: 把 routing 字段持久化到 usage_logs 与 ops_error_logs

**Files:**
- Modify: `backend/ent/schema/usage_log.go`
- Create: `backend/migrations/082_add_openai_routing_observability.sql`
- Modify: `backend/internal/service/usage_log.go`
- Modify: `backend/internal/service/ops_port.go`
- Modify: `backend/internal/service/openai_gateway_service.go`
- Modify: `backend/internal/handler/ops_error_logger.go`
- Modify: `backend/internal/repository/usage_log_repo.go`
- Modify: `backend/internal/repository/usage_log_repo_request_type_test.go`
- Modify: `backend/internal/repository/ops_repo.go`
- Modify: `backend/internal/service/ops_service_batch_test.go`

- [ ] **Step 1: 先写失败测试，锁定 usage 和 ops error 都能承接 routing 字段**

```go
func TestUsageLogRepo_InsertIncludesOpenAIRoutingFields(t *testing.T) {
	log := &service.UsageLog{RequestID: "req_routing_1", Model: "gpt-5.4", RequestedModel: "gpt-5.4-Sys", RoutingTargetGroup: strPtr("exhausted"), RoutingScheduleLayer: strPtr("load_balance"), RoutingSelectedAccountID: i64Ptr(66), RoutingSelectedAccountName: strPtr("acc-66"), RoutingEffectiveModel: strPtr("gpt-5.4"), RoutingFailoverCount: iPtr(1), RoutingFailoverFinalReason: strPtr("selected_exhausted_fallback")}
	// 断言 INSERT 参数包含 routing 列
}

func TestOpsService_RecordError_PersistsOpenAIRoutingFields(t *testing.T) {
	entry := &OpsInsertErrorLogInput{RequestID: "req_err", Platform: "openai", Model: "gpt-5.4", RoutingTargetGroup: "active", RoutingScheduleLayer: "sticky_session", RoutingSelectedAccountID: i64Ptr(64), RoutingSelectedAccountName: strPtr("acc-64"), RoutingRequestedModel: "gpt-5.4", RoutingEffectiveModel: "gpt-5.4", RoutingFailoverCount: 2, RoutingFailoverFinalReason: "upstream_502"}
	// 断言 opsRepo.InsertErrorLog 收到 routing 字段
}
```

- [ ] **Step 2: 运行失败测试**

Run: `go test ./internal/repository -run "TestUsageLogRepo_InsertIncludesOpenAIRoutingFields" -count=1`
Expected: FAIL，当前 insert/select 还没有这些列。

Run: `go test ./internal/service -run "TestOpsService_RecordError_PersistsOpenAIRoutingFields" -count=1`
Expected: FAIL，当前 `OpsInsertErrorLogInput` 还没有这些字段。

- [ ] **Step 3: 给服务模型、ent schema、migration 增加 routing 字段**

```go
// usage_log.go
RoutingTargetGroup         *string
RoutingScheduleLayer       *string
RoutingSelectedAccountID   *int64
RoutingSelectedAccountName *string
RoutingEffectiveModel      *string
RoutingFailoverCount       *int
RoutingFailoverFinalReason *string
```

```sql
ALTER TABLE usage_logs
  ADD COLUMN routing_target_group text,
  ADD COLUMN routing_schedule_layer text,
  ADD COLUMN routing_selected_account_id bigint,
  ADD COLUMN routing_selected_account_name text,
  ADD COLUMN routing_effective_model text,
  ADD COLUMN routing_failover_count integer,
  ADD COLUMN routing_failover_final_reason text;

ALTER TABLE ops_error_logs
  ADD COLUMN routing_target_group text,
  ADD COLUMN routing_schedule_layer text,
  ADD COLUMN routing_selected_account_id bigint,
  ADD COLUMN routing_selected_account_name text,
  ADD COLUMN routing_requested_model text,
  ADD COLUMN routing_effective_model text,
  ADD COLUMN routing_failover_count integer,
  ADD COLUMN routing_failover_final_reason text;
```

- [ ] **Step 4: 把 snapshot 接到成功写 usage、失败写 ops error 的位置**

```go
usageLog.RoutingTargetGroup = optionalStringPtr(snapshot.TargetGroup)
usageLog.RoutingScheduleLayer = optionalStringPtr(snapshot.ScheduleLayer)
usageLog.RoutingSelectedAccountID = snapshot.SelectedAccountID
usageLog.RoutingSelectedAccountName = snapshot.SelectedAccountName
usageLog.RoutingEffectiveModel = optionalStringPtr(snapshot.EffectiveModel)
usageLog.RoutingFailoverCount = optionalIntPtr(snapshot.FailoverCount)
usageLog.RoutingFailoverFinalReason = optionalStringPtr(snapshot.FailoverFinalReason)
```

```go
entry.RoutingTargetGroup = snapshot.TargetGroup
entry.RoutingScheduleLayer = snapshot.ScheduleLayer
entry.RoutingSelectedAccountID = snapshot.SelectedAccountID
entry.RoutingSelectedAccountName = snapshot.SelectedAccountName
entry.RoutingRequestedModel = snapshot.RequestedModel
entry.RoutingEffectiveModel = snapshot.EffectiveModel
entry.RoutingFailoverCount = snapshot.FailoverCount
entry.RoutingFailoverFinalReason = snapshot.FailoverFinalReason
```

- [ ] **Step 5: 跑相关测试并确认持久化通过**

Run: `go test ./internal/repository -run "TestUsageLogRepo_InsertIncludesOpenAIRoutingFields" -count=1`
Expected: PASS。

Run: `go test ./internal/service -run "TestOpsService_RecordError_PersistsOpenAIRoutingFields" -count=1`
Expected: PASS。

- [ ] **Step 6: Commit**

```bash
git add backend/ent/schema/usage_log.go backend/migrations/082_add_openai_routing_observability.sql backend/internal/service/usage_log.go backend/internal/service/ops_port.go backend/internal/service/openai_gateway_service.go backend/internal/handler/ops_error_logger.go backend/internal/repository/usage_log_repo.go backend/internal/repository/usage_log_repo_request_type_test.go backend/internal/repository/ops_repo.go backend/internal/service/ops_service_batch_test.go
git commit -m "feat(openai): 持久化路由观测字段"
```

### Task 3: 让 admin usage 与 request details 返回 routing 字段并支持过滤

**Files:**
- Modify: `backend/internal/handler/dto/types.go`
- Modify: `backend/internal/handler/dto/mappers.go`
- Modify: `backend/internal/service/ops_request_details.go`
- Modify: `backend/internal/repository/usage_log_repo.go`
- Modify: `backend/internal/repository/ops_repo.go`
- Create: `backend/internal/service/ops_request_details_test.go`
- Modify: `frontend/src/types/index.ts`
- Modify: `frontend/src/api/admin/ops.ts`
- Modify: `frontend/src/views/admin/UsageView.vue`
- Modify: `frontend/src/views/admin/ops/components/OpsRequestDetailsModal.vue`

- [ ] **Step 1: 写失败测试，锁定 usage DTO 和 request details 会暴露 routing 字段**

```go
func TestUsageLogFromServiceAdmin_IncludesRoutingSnapshot(t *testing.T) {
	log := &service.UsageLog{RequestID: "req_1", RoutingTargetGroup: strPtr("exhausted"), RoutingScheduleLayer: strPtr("load_balance"), RoutingSelectedAccountName: strPtr("acc-66")}
	dto := UsageLogFromServiceAdmin(log)
	if dto.RoutingTargetGroup == nil || *dto.RoutingTargetGroup != "exhausted" { t.Fatalf("missing target group") }
}
```

- [ ] **Step 2: 跑失败测试**

Run: `go test ./internal/handler/dto -run TestUsageLogFromServiceAdmin_IncludesRoutingSnapshot -count=1`
Expected: FAIL。

- [ ] **Step 3: 扩展 DTO、repo filter 和前端 usage/request-details 类型**

```ts
export interface AdminUsageLog extends UsageLog {
  routing_target_group?: string | null
  routing_schedule_layer?: string | null
  routing_selected_account_name?: string | null
  routing_effective_model?: string | null
  routing_failover_count?: number | null
  routing_failover_final_reason?: string | null
}
```

- [ ] **Step 4: 在 UsageView 增加列、过滤项和导出字段，在 OpsRequestDetailsModal 增加 routing 列**

```ts
{ key: 'routing_target_group', label: 'Target Group', sortable: false },
{ key: 'routing_schedule_layer', label: 'Schedule Layer', sortable: false },
{ key: 'routing_account', label: 'Routed Account', sortable: false },
```

- [ ] **Step 5: 跑测试与前端构建**

Run: `go test ./internal/handler/dto -count=1`
Expected: PASS。

Run: `pnpm test:run "src/views/admin/**"`
Expected: PASS（至少新增/修改的 usage/request-details 相关 spec 通过）。

- [ ] **Step 6: Commit**

```bash
git add backend/internal/handler/dto/types.go backend/internal/handler/dto/mappers.go backend/internal/service/ops_request_details.go backend/internal/repository/usage_log_repo.go backend/internal/repository/ops_repo.go backend/internal/service/ops_request_details_test.go frontend/src/types/index.ts frontend/src/api/admin/ops.ts frontend/src/views/admin/UsageView.vue frontend/src/views/admin/ops/components/OpsRequestDetailsModal.vue
git commit -m "feat(ops): 暴露请求级路由观测字段"
```

### Task 4: 新增 OpenAI routing 聚合接口

**Files:**
- Create: `backend/internal/service/ops_openai_routing_stats.go`
- Create: `backend/internal/service/ops_openai_routing_stats_test.go`
- Modify: `backend/internal/repository/ops_repo.go`
- Modify: `backend/internal/service/ops_port.go`
- Modify: `backend/internal/handler/admin/ops_dashboard_handler.go`
- Modify: `backend/internal/server/routes/admin.go`
- Modify: `frontend/src/api/admin/ops.ts`

- [ ] **Step 1: 写失败测试，锁定 active/exhausted 的次数与 token 聚合**

```go
func TestGetOpenAIRoutingStats_GroupsByTargetGroup(t *testing.T) {
	// 构造 usage_logs 聚合结果：active 10 req / exhausted 4 req / input / output / total tokens
	// 断言 service 返回 counts、total tokens、input tokens、output tokens 四类统计
}
```

- [ ] **Step 2: 跑失败测试**

Run: `go test ./internal/service -run TestGetOpenAIRoutingStats_GroupsByTargetGroup -count=1`
Expected: FAIL。

- [ ] **Step 3: 先定义返回模型，再补 repo 聚合 SQL 与 handler 路由**

```go
type OpsOpenAIRoutingStats struct {
	RequestCountByGroup map[string]int64
	TotalTokensByGroup  map[string]int64
	InputTokensByGroup  map[string]int64
	OutputTokensByGroup map[string]int64
}
```

- [ ] **Step 4: 注册新接口**

```go
ops.GET("/dashboard/openai-routing", h.Admin.Ops.GetOpenAIRoutingStats)
```

- [ ] **Step 5: 跑 service/handler 测试**

Run: `go test ./internal/service -run TestGetOpenAIRoutingStats_GroupsByTargetGroup -count=1`
Expected: PASS。

- [ ] **Step 6: Commit**

```bash
git add backend/internal/service/ops_openai_routing_stats.go backend/internal/service/ops_openai_routing_stats_test.go backend/internal/repository/ops_repo.go backend/internal/service/ops_port.go backend/internal/handler/admin/ops_dashboard_handler.go backend/internal/server/routes/admin.go frontend/src/api/admin/ops.ts
git commit -m "feat(ops): 增加 OpenAI 路由聚合接口"
```

### Task 5: 在 Ops Dashboard 展示 OpenAI routing 分布卡片

**Files:**
- Create: `frontend/src/views/admin/ops/components/OpsOpenAIRoutingCard.vue`
- Create: `frontend/src/views/admin/ops/components/__tests__/OpsOpenAIRoutingCard.spec.ts`
- Modify: `frontend/src/views/admin/ops/OpsDashboard.vue`
- Modify: `frontend/src/i18n/locales/zh.ts`
- Modify: `frontend/src/i18n/locales/en.ts`

- [ ] **Step 1: 写失败测试，锁定卡片要展示 4 组指标**

```ts
it('renders active/exhausted request and token distributions', () => {
  // 断言页面上出现 requests / total tokens / input tokens / output tokens 四块内容
})
```

- [ ] **Step 2: 运行失败测试**

Run: `pnpm test:run "src/views/admin/ops/components/__tests__/OpsOpenAIRoutingCard.spec.ts"`
Expected: FAIL，组件尚不存在。

- [ ] **Step 3: 写最小实现并挂到 OpsDashboard**

```vue
<OpsOpenAIRoutingCard
  v-if="opsEnabled"
  :platform-filter="platformFilter"
  :group-id-filter="groupIdFilter"
  :time-range="selectedTimeRange"
  :start-time="customRange.start"
  :end-time="customRange.end"
/>
```

- [ ] **Step 4: 跑前端测试和构建**

Run: `pnpm test:run "src/views/admin/ops/components/__tests__/OpsOpenAIRoutingCard.spec.ts"`
Expected: PASS。

Run: `pnpm build`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add frontend/src/views/admin/ops/components/OpsOpenAIRoutingCard.vue frontend/src/views/admin/ops/components/__tests__/OpsOpenAIRoutingCard.spec.ts frontend/src/views/admin/ops/OpsDashboard.vue frontend/src/i18n/locales/zh.ts frontend/src/i18n/locales/en.ts
git commit -m "feat(frontend): 展示 OpenAI 路由分布卡片"
```

### Task 6: 端到端验证与收尾

**Files:**
- Modify: `backend/internal/service/openai_routing_observability_test.go`
- Modify: `backend/internal/service/ops_openai_routing_stats_test.go`
- Modify: `frontend/src/views/admin/ops/components/__tests__/OpsOpenAIRoutingCard.spec.ts`
- Modify: `frontend/src/views/admin/UsageView.vue`
- Modify: `frontend/src/views/admin/ops/components/OpsRequestDetailsModal.vue`

- [ ] **Step 1: 跑后端相关包回归**

Run: `go test ./internal/service -count=1`
Expected: PASS。

Run: `go test ./internal/repository -count=1`
Expected: PASS。

Run: `go test ./internal/handler/... -count=1`
Expected: PASS。

- [ ] **Step 2: 跑前端相关测试与构建**

Run: `pnpm test:run`
Expected: PASS 或至少新增受影响测试全部 PASS。

Run: `pnpm build`
Expected: PASS。

- [ ] **Step 3: 做本机运行实例验证**

Run:

```bash
go build -tags embed -o C:\Users\34404\sub2api-runtime\app\sub2api.routing-observability.new.exe ./cmd/server
```

然后重启实例并验证：

1. 发送至少一条 `active` 请求和一条 `exhausted` 请求。
2. 在 `usage_logs` / admin usage API 中看到 `routing_target_group` 与 `routing_schedule_layer`。
3. 在 `ops request details` 中看到同构字段。
4. 在新的 ops routing 卡片中看到 active/exhausted 次数和 token 分布。

- [ ] **Step 4: 运行格式化与 diff 检查**

Run: `git diff --check`
Expected: 无输出。

- [ ] **Step 5: Commit**

```bash
git add .
git commit -m "feat(openai): 增加路由观测与分布统计"
```

## 自查

- spec 覆盖：本计划覆盖了持久化字段、usage/request details 展示、ops routing 聚合、前端 dashboard 展示和测试验证，没有遗漏你确认过的三种聚合口径。
- 占位检查：没有遗留待填占位词；每个任务都给了目标文件、失败测试、验证命令和预期。
- 类型一致性：计划统一使用 `routing_target_group`、`routing_schedule_layer`、`routing_selected_account_*`、`routing_effective_model`、`routing_failover_*` 这一组命名，后续任务不再改名。
