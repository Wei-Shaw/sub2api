# OpenAI Reserve Group Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在 OpenAI 调度主链中引入 exhausted-class overflow `reserve` 子组，在不打破现有 `active/exhausted/-Sys` guardrail 的前提下，为 exhausted 高压阶段提供动态备用容量池，并把 reserve 命中结果打通到写库、ops 面板和 request details。

**Architecture:** 保持请求语义目标组仍是 `active/exhausted/any`，新增 `reserve` 作为 exhausted-class 内部的实际命中子组。调度层负责 reserve 池形成和导流；OpenAI 专用亲和 carrier 承载 `affinity_domain + selected_group`；观测层新增 `routing_selected_group`，routing/retry 统计继续按 exhausted-class 请求过滤，再按实际命中子组展示。

**Tech Stack:** Go, Gin, PostgreSQL, Ent schema + SQL repositories, Vue 3, TypeScript, Vitest

---

### Task 1: 建立 reserve overlay 选择层与目标组判定基础

**Files:**
- Modify: `backend/internal/service/account.go`
- Modify: `backend/internal/service/openai_gateway_service.go`
- Modify: `backend/internal/service/openai_gateway_service_test.go`

- [ ] **Step 1: 写 reserve 基础判定红灯测试**

在 `openai_gateway_service_test.go` 增加最小失败测试，覆盖：

1. exhausted 自身配置容量不足时，能从 free active 候选中补 reserve 容量直到达到 60% 目标线
2. exhausted 使用率 `<= 60%` 时，请求不进入 reserve
3. exhausted 使用率 `> 60%` 时，溢出请求开始进入 reserve

建议新增测试名：

```go
func TestBuildReserveCandidatePool_FillsToSixtyPercent(t *testing.T)
func TestShouldUseReserveForExhaustedOverflow_BelowThreshold(t *testing.T)
func TestShouldUseReserveForExhaustedOverflow_AboveThreshold(t *testing.T)
```

- [ ] **Step 2: 运行测试确认当前失败**

Run: `go test ./internal/service -run "ReserveCandidatePool|UseReserveForExhaustedOverflow" -count=1`

Expected: FAIL，当前代码没有 reserve 组或 reserve 判定逻辑。

- [ ] **Step 3: 在 account / OpenAI source 层增加 reserve 基础能力**

在 `backend/internal/service/account.go` 增加最小 helper，不引入全局第三状态：

```go
func (a *Account) IsOpenAIReserveCandidate() bool {
	if a == nil {
		return false
	}
	if a.Platform != PlatformOpenAI || a.Type != AccountTypeOAuth {
		return false
	}
	if a.IsExhausted() || !a.IsSchedulable() {
		return false
	}
	return strings.EqualFold(a.GetCredential("plan_type"), "free")
}

func (a *Account) OpenAIRemainingQuotaScore() float64 {
	if a == nil {
		return -1
	}
	used7d, ok7d := a.GetCodex7dUsedPercent()
	usedPrimary, okPrimary := a.GetCodexPrimaryUsedPercent()
	if ok7d {
		return 100 - used7d
	}
	if okPrimary {
		return 100 - usedPrimary
	}
	return -1
}
```

然后在 `backend/internal/service/openai_gateway_service.go` 新增 reserve 选择基础 helper，例如：

```go
func buildOpenAIReserveCandidatePool(accounts []Account) []Account
func calculateOpenAIConcurrentCapacity(accounts []Account) int
func buildOpenAIReservePool(activeAccounts []Account, exhaustedAccounts []Account) []Account
func shouldRouteExhaustedOverflowToReserve(exhaustedAccounts []Account, reserveAccounts []Account) bool
```

要求：
- reserve 候选只从 `IsOpenAIReserveCandidate()` 过滤出来
- reserve 池大小只补到 60% 目标线
- exhausted 使用率阈值按并发容量占比算

- [ ] **Step 4: 重跑定向测试**

Run: `go test ./internal/service -run "ReserveCandidatePool|UseReserveForExhaustedOverflow" -count=1`

Expected: PASS

- [ ] **Step 5: 提交 Task 1**

```bash
git add backend/internal/service/account.go backend/internal/service/openai_gateway_service.go backend/internal/service/openai_gateway_service_test.go
git commit -m "feat(openai): 增加 reserve 候选池与导流基础逻辑"
```

### Task 2: 在 scheduler 中接入 reserve 溢出选择，并保持 exhausted-class guardrail

**Files:**
- Modify: `backend/internal/service/openai_account_scheduler.go`
- Modify: `backend/internal/service/openai_account_scheduler_test.go`
- Modify: `backend/internal/service/openai_gateway_service.go`
- Modify: `backend/internal/handler/openai_gateway_handler.go`
- Modify: `backend/internal/handler/openai_chat_completions.go`
- Test: `backend/internal/handler/openai_gateway_handler_test.go`

- [ ] **Step 1: 写红灯测试，锁定 exhausted-class overflow 选择**

在 `openai_account_scheduler_test.go` 增加测试：

```go
func TestSelectByLoadBalance_ExhaustedOverflowUsesReserve(t *testing.T)
func TestSelectByLoadBalance_ExhaustedBelowThresholdDoesNotUseReserve(t *testing.T)
func TestSelectByLoadBalance_ReserveBelowThresholdAbsorbsOverflowFirst(t *testing.T)
func TestSelectByLoadBalance_ReserveAtThresholdThenCoSchedulesWithExhausted(t *testing.T)
func TestReservePoolReleasesHighestRemainingQuotaBackToActive(t *testing.T)
```

断言：
- exhausted 使用率未超过 60% 时，不会选 reserve
- exhausted 使用率超过 60% 时，reserve 可以被选中
- reserve 利用率仍低于 60% 时，新增 overflow 优先落 reserve
- reserve 也达到 60% 后，exhausted / reserve 再共同参与选择
- reserve 超额回收时，优先移回剩余配额最多的账号

- [ ] **Step 2: 运行测试确认当前失败**

Run: `go test ./internal/service -run "ExhaustedOverflowUsesReserve|BelowThresholdDoesNotUseReserve|ReserveBelowThresholdAbsorbsOverflowFirst|ReserveAtThresholdThenCoSchedulesWithExhausted|ReleasesHighestRemainingQuota" -count=1`

Expected: FAIL

- [ ] **Step 3: 实现 scheduler reserve 导流与回收**

在 `openai_account_scheduler.go`：

1. 扩展调度决策，但不新增请求语义 target group：

```go
type OpenAIAccountScheduleDecision struct {
	...
	SelectedGroup string // active / exhausted / reserve
}
```

并同步扩展 `OpenAIRoutingSnapshot` / `OpenAIRoutingSnapshotInput` / clone helper，使 `routing_selected_group` 先通过 snapshot carrier 稳定传递，而不是在写库末端按账号状态反推。

2. 在 exhausted-class load-balance 选择阶段插入 reserve overlay 判断：
- 请求语义仍然是 `TargetGroupExhausted`
- exhausted 使用率 `> 60%` 时，进入 overflow 调控
- overflow 调控细则必须写死为：
  - reserve 当前利用率 `< 60%` 时，新增 overflow 优先只落 reserve
  - reserve 当前利用率 `>= 60%` 后，exhausted 与 reserve 再共同参与选择
- `SelectedGroup` 记录最终实际命中的 subgroup

**硬约束：**
- 不得修改共享 `Account.MatchesTargetGroup`、`Account.IsSchedulableForTargetGroup`、generic previous-response restriction 的既有语义
- reserve 只能通过 OpenAI exhausted overlay 决策层进入，不能把 shared helper 放宽成“active 账号也算 exhausted”

3. `TargetGroupAny` 路径保持既有行为，不引入 reserve 候选。

4. 如果 reserve 被命中，`SelectedGroup = "reserve"`，但请求 guardrail 语义仍然保持 exhausted-class。

- [ ] **Step 4: 在 `openai_gateway_service.go` 接通实际 reserve 来源**

让 `OpenAIGatewayService` 为 scheduler 提供：
- exhausted base accounts
- reserve overlay accounts
- 当前 exhausted 使用率判定所需数据

但不要把 `reserve` 变成新的 `TargetGroup` 请求入口。

- [ ] **Step 5: 重跑定向测试**

Run: `go test ./internal/service -run "ExhaustedOverflowUsesReserve|BelowThresholdDoesNotUseReserve|ReserveBelowThresholdAbsorbsOverflowFirst|ReserveAtThresholdThenCoSchedulesWithExhausted|ReleasesHighestRemainingQuota" -count=1`

Expected: PASS

- [ ] **Step 6: 提交 Task 2**

```bash
git add backend/internal/service/openai_account_scheduler.go backend/internal/service/openai_account_scheduler_test.go backend/internal/service/openai_gateway_service.go
git commit -m "feat(openai): 在 exhausted 高压时导向 reserve 子组"
```

### Task 3: 新增 Redis 持久化的 OpenAI 专用亲和 carrier，保证 reserve continuation 仍按 exhausted-class 命中

**Files:**
- Modify: `backend/internal/service/gateway_service.go`
- Modify: `backend/internal/service/openai_account_scheduler.go`
- Modify: `backend/internal/service/openai_routing_observability.go`
- Modify: `backend/internal/service/openai_sticky_compat.go`
- Modify: `backend/internal/service/openai_ws_state_store.go`
- Modify: `backend/internal/service/openai_ws_forwarder.go`
- Modify: `backend/internal/service/openai_gateway_service.go`
- Modify: `backend/internal/repository/gateway_cache.go`
- Modify: `backend/internal/handler/openai_gateway_handler.go`
- Modify: `backend/internal/handler/openai_chat_completions.go`
- Test: `backend/internal/service/openai_account_scheduler_test.go`
- Test: `backend/internal/service/openai_ws_account_sticky_test.go`
- Test: `backend/internal/handler/openai_gateway_handler_test.go`

- [ ] **Step 1: 写红灯测试，锁 reserve 命中后的 exhausted-class 续链**

至少增加：

```go
func TestStickyReserveBindingStillMatchesExhaustedClass(t *testing.T)
func TestPreviousResponseReserveBindingStillMatchesExhaustedClass(t *testing.T)
```

断言：
- exhausted 请求命中 reserve 账号后
- 下一轮 exhausted-class sticky / previous_response continuation 仍能命中
- 不会落成 `miss_binding_restricted`

- [ ] **Step 2: 运行测试确认当前失败**

Run: `go test ./internal/service ./internal/handler -run "ReserveBinding|ExhaustedClass" -count=1`

Expected: FAIL

- [ ] **Step 3: 扩 OpenAI 专用 binding carrier（不改 shared GatewayCache）**

第一阶段明确：
- 保留现有通用 `sessionHash -> accountID` / `response_id -> accountID` 协议含义不变
- 但要在现有 Redis/cache 体系上新增 **OpenAI 专用 companion binding**，不能只用进程内 map
- companion binding 承载结构例如：

```go
type openAIAffinityBinding struct {
	BoundAccountID int64  `json:"bound_account_id"`
	AffinityDomain string `json:"affinity_domain"`   // active / exhausted
	SelectedGroup  string `json:"selected_group"`    // active / exhausted / reserve
}
```

要求：
- exhausted 命中 reserve 时，binding 里写 `affinity_domain=exhausted, selected_group=reserve`
- 后续 sticky / previous_response 先按 `affinity_domain` 判断 exhausted-class，再决定命中，而不是直接按账号固有 active-class 属性 mismatch
- companion binding 必须跨请求、跨进程、跨实例可恢复；不能退化成只在当前进程有效
- 旧的 shared `accountID` binding 继续保留，用于不需要 reserve 语义的既有路径

特别要求把 `previous_response_id` 的真实实现位点和现成 harness 一起纳入：
- `openai_ws_state_store.go`
- `openai_ws_forwarder.go`
- `openai_ws_account_sticky_test.go`

- [ ] **Step 4: handler / snapshot 传递更新**

在 `openai_gateway_handler.go`、`openai_chat_completions.go` 中，确保：
- `routing_target_group` 继续记录请求语义 exhausted
- `routing_selected_group` 记录实际命中 reserve
- sticky / previous_response 观测字段继续可用

- [ ] **Step 5: 重跑定向测试**

Run: `go test ./internal/service ./internal/handler -run "ReserveBinding|ExhaustedClass|RoutingSnapshot" -count=1`

Expected: PASS

- [ ] **Step 6: 提交 Task 3**

```bash
git add backend/internal/service/openai_account_scheduler.go backend/internal/service/openai_routing_observability.go backend/internal/service/openai_sticky_compat.go backend/internal/service/openai_ws_state_store.go backend/internal/service/openai_ws_forwarder.go backend/internal/service/openai_gateway_service.go backend/internal/handler/openai_gateway_handler.go backend/internal/handler/openai_chat_completions.go backend/internal/service/openai_account_scheduler_test.go backend/internal/service/openai_ws_account_sticky_test.go backend/internal/handler/openai_gateway_handler_test.go
git commit -m "feat(openai): 为 reserve 增加 exhausted-class 亲和载体"
```

### Task 4: 打通 `routing_selected_group` carrier 到 usage / ops / request-details / frontend types

**Files:**
- Modify: `backend/internal/service/usage_log.go`
- Modify: `backend/internal/service/ops_port.go`
- Modify: `backend/internal/service/openai_routing_observability.go`
- Modify: `backend/internal/service/openai_gateway_service.go`
- Test: `backend/internal/service/openai_gateway_record_usage_test.go`
- Modify: `backend/ent/schema/usage_log.go`
- Modify: migration / ent generated carriers as needed
- Modify: `backend/migrations/*`（新增/更新 `routing_selected_group` 相关 migration）
- Modify: `backend/internal/repository/usage_log_repo.go`
- Modify: `backend/internal/repository/ops_repo.go`
- Modify: `backend/internal/repository/ops_repo_request_details.go`
- Modify: `backend/internal/handler/ops_error_logger.go`
- Modify: `backend/internal/handler/dto/types.go`
- Modify: `backend/internal/handler/dto/mappers.go`
- Modify: `backend/internal/service/ops_request_details.go`
- Modify: `backend/internal/handler/admin/usage_handler.go`
- Modify: `frontend/src/api/admin/ops.ts`
- Modify: `frontend/src/types/index.ts`

- [ ] **Step 1: 写红灯测试，锁 `routing_selected_group` carrier**

至少覆盖：
- `usage_logs` success 链带 `routing_selected_group`
- `ops_error_logs` error 链带 `routing_selected_group`
- request details 能读出并按 `routing_selected_group` 过滤

Run: `go test ./internal/repository ./internal/handler ./internal/service -run "RoutingSelectedGroup|RequestDetails|RecordUsage" -count=1`

Expected: FAIL

- [ ] **Step 2: 实现新字段链路**

要求：
- 新增 `routing_selected_group`
- `routing_target_group` 保留请求语义
- `routing_selected_group` 对非 reserve 命中也稳定写 `active/exhausted`
- request-details / retry drilldown 能按 `routing_selected_group` 过滤

同时明确 success-chain carrier 必须通过：
- `OpenAIRoutingSnapshot`
- `OpenAIGatewayService` usage writeback
- DTO mapper / admin usage API

不允许在写库末端按账号当前状态反推 `routing_selected_group`。

- [ ] **Step 3: 重跑定向测试**

Run: `go test ./internal/repository ./internal/handler ./internal/service -run "RoutingSelectedGroup|RequestDetails|RecordUsage" -count=1`

Expected: PASS

- [ ] **Step 4: 提交 Task 4**

```bash
git add backend/internal/service/usage_log.go backend/internal/service/ops_port.go backend/ent/schema/usage_log.go backend/internal/repository/usage_log_repo.go backend/internal/repository/ops_repo.go backend/internal/repository/ops_repo_request_details.go backend/internal/handler/ops_error_logger.go backend/internal/handler/dto/types.go backend/internal/service/ops_request_details.go frontend/src/api/admin/ops.ts frontend/src/types/index.ts
git commit -m "feat(ops): 打通 reserve 实际命中组观测链"
```

### Task 5: 扩展 routing / retry / usage 展示到 reserve

**Files:**
- Modify: `backend/internal/service/ops_openai_routing_stats.go`
- Modify: `backend/internal/repository/ops_repo_openai_routing_stats.go`
- Modify: `backend/internal/handler/admin/ops_handler.go`
- Modify: `backend/internal/service/ops_request_details.go`
- Modify: `frontend/src/views/admin/ops/components/OpsOpenAIRoutingCard.vue`
- Modify: `frontend/src/views/admin/ops/components/OpsOpenAIRetryCard.vue`
- Modify: `frontend/src/views/admin/ops/components/OpsOpenAIStickyCard.vue`
- Modify: `frontend/src/views/admin/ops/components/OpsRequestDetailsModal.vue`
- Modify: `frontend/src/views/admin/ops/components/OpsOpenAIRetryDetailsModal.vue`
- Modify: `frontend/src/views/admin/ops/OpsDashboard.vue`
- Modify: `frontend/src/components/admin/usage/UsageTable.vue`
- Modify: `frontend/src/views/admin/UsageView.vue`
- Modify: 相关 i18n

- [ ] **Step 1: 写前后端红灯测试**

至少覆盖：
- routing/retry stats 继续只统计 `routing_target_group in ('active','exhausted')`，但按 `routing_selected_group` 展示 reserve
- reserve 卡片/drilldown 能点出 reserve 明细
- usage/request details 中能看见 reserve badge/字段

- [ ] **Step 2: 实现卡片与 drilldown 扩展**

要求：
- routing card / retry card 继续 exhausted-class 请求过滤
- 但展示维度新增 reserve
- reserve drilldown 走 `routing_selected_group` 参数，而不是旧的 `routing_target_group`

- [ ] **Step 3: 前端验证**

Run:
- `pnpm typecheck`
- 相关 OpenAI ops/retry/sticky/request-details Vitest

Expected: PASS

- [ ] **Step 4: 提交 Task 5**

```bash
git add backend/internal/service/ops_openai_routing_stats.go backend/internal/repository/ops_repo_openai_routing_stats.go backend/internal/handler/admin/ops_handler.go frontend/src/views/admin/ops/components/OpsOpenAIRoutingCard.vue frontend/src/views/admin/ops/components/OpsOpenAIRetryCard.vue frontend/src/views/admin/ops/components/OpsRequestDetailsModal.vue frontend/src/views/admin/ops/components/OpsOpenAIRetryDetailsModal.vue frontend/src/views/admin/ops/OpsDashboard.vue frontend/src/components/admin/usage/UsageTable.vue frontend/src/views/admin/UsageView.vue frontend/src/api/admin/ops.ts frontend/src/types/index.ts
git commit -m "feat(ops): 扩展 reserve 分组展示与 drilldown"
```

### Task 6: 全量验证与收尾

**Files:**
- Verify current changes only

- [ ] **Step 1: 跑后端全量验证**

Run:
- `go test ./internal/handler ./internal/repository ./internal/server/... -count=1`
- `go test -tags unit ./internal/service ./internal/pkg/apicompat -count=1`
- `go build ./cmd/server`

Expected: PASS

- [ ] **Step 2: 跑前端验证**

Run:
- `pnpm typecheck`
- 相关 ops / usage / request-details Vitest

Expected: PASS

- [ ] **Step 3: 人工边界确认**

确认：
- reserve 不是新的请求 target group
- `routing_target_group` 仍保留 exhausted-class guardrail 语义
- `routing_selected_group` 正确记录实际命中子组
- exhausted continuation 命中 reserve 后仍能正常续链
- routing/retry 卡片和明细不再只认 `active/exhausted`

- [ ] **Step 4: 提交 Task 6 / 收尾提交**

```bash
git add .
git commit -m "feat(openai): 引入 exhausted reserve overflow 调度子组"
```
