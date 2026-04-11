# OpenAI Session Affinity 与 Sticky 可观测性 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让 OpenAI HTTP 主链正式消费 OpenCode 的 `x-session-affinity` 作为 sticky 会话信号，并新增一条可追请求级细节、可聚合到运维面板的 sticky/session 观测链。

**Architecture:** 本轮分成 4 层闭环：1) 统一 session 信号提取并同步到 upstream cache/session 注入；2) 在 handler/service 调度链里生成 sticky 观测结果并通过 context 传递；3) 在 success/error 双链同时持久化，并新增独立的 `/admin/ops/dashboard/openai-sticky` 聚合接口；4) 前端新增 sticky 卡片并在 request details 弹窗里展示新字段。整个实现只覆盖 OpenAI HTTP 主链（`/v1/responses`、`/v1/chat/completions`），不引入 prefix-hash 兜底，也不修改 failover/guardrail 规则。

**Tech Stack:** Go, Gin, PostgreSQL, Ent schema + hand-written SQL repositories, Vue 3, TypeScript, Vitest

---

## File Structure

- `backend/internal/service/openai_gateway_service.go`
  - 统一提取 `session_id / conversation_id / x-session-affinity / prompt_cache_key / content_fallback`
  - 同步 sticky hash 和 upstream cache/session 注入逻辑
- `backend/internal/service/openai_gateway_service_test.go`
  - session 信号优先级、upstream cache/session 注入测试
- `backend/internal/service/openai_routing_observability.go`
  - 扩展 `OpenAIRoutingSnapshot` / `OpenAIRoutingSnapshotInput`，承载 sticky 观测字段
- `backend/internal/service/openai_account_scheduler.go`
  - 生成“首次 sticky 判定结果”及 miss 原因的真实来源
- `backend/internal/service/openai_account_scheduler_test.go`
  - sticky 判定结果与 miss reason 回归测试
- `backend/internal/handler/openai_gateway_handler.go`
  - `/v1/responses` 路径生成并写入 sticky 观测摘要
- `backend/internal/handler/openai_chat_completions.go`
  - `/v1/chat/completions` 路径生成并写入 sticky 观测摘要
- `backend/internal/service/usage_log.go`
  - 成功链字段定义
- `backend/ent/schema/usage_log.go`
  - `usage_logs` 新字段 schema
- `backend/migrations/092_add_openai_sticky_observability.sql`
  - `usage_logs` / `ops_error_logs` 新字段 migration
- `backend/internal/repository/usage_log_repo.go`
  - `usage_logs` insert/select/scan 列顺序扩展
- `backend/internal/service/ops_port.go`
  - `OpsInsertErrorLogInput` 新字段定义
- `backend/internal/handler/ops_error_logger.go`
  - 失败链 sticky 观测写入
- `backend/internal/repository/ops_repo.go`
  - `ops_error_logs` insert SQL 扩展
- `backend/internal/service/ops_request_details.go`
  - request details DTO 扩展
- `backend/internal/repository/ops_repo_request_details.go`
  - success/error 联合细节读取扩展
- `backend/internal/service/ops_port.go`
  - 新 sticky stats repo/service 接口声明
- `backend/internal/service/ops_repo_mock_test.go`
  - `OpsRepository` mock/stub 同步新方法
- `backend/internal/service/ops_openai_sticky_stats.go`
  - 新 sticky 聚合 response/service 封装
- `backend/internal/repository/ops_repo_openai_sticky_stats.go`
  - 新 sticky 聚合 SQL
- `backend/internal/handler/admin/ops_dashboard_handler.go`
  - 新 sticky stats handler
- `backend/internal/server/routes/admin.go`
  - 注册 `/admin/ops/dashboard/openai-sticky`
- `frontend/src/api/admin/ops.ts`
  - sticky stats API 类型 + request details 类型扩展
- `frontend/src/views/admin/ops/components/OpsOpenAIStickyCard.vue`
  - 新 sticky 卡片
- `frontend/src/views/admin/ops/components/OpsRequestDetailsModal.vue`
  - 展示 session/sticky 字段
- `frontend/src/views/admin/ops/OpsDashboard.vue`
  - 挂载新卡片
- `frontend/src/i18n/locales/en.ts`
- `frontend/src/i18n/locales/zh.ts`
  - 新卡片与 request details 文案

### Task 1: 统一 session 信号提取，并同步到 sticky 与 upstream cache/session 注入

**Files:**
- Modify: `backend/internal/service/openai_gateway_service.go`
- Modify: `backend/internal/service/openai_gateway_service_test.go`

- [ ] **Step 1: 写真正会失败的优先级测试**

```go
func TestGenerateSessionHash_UsesXSessionAffinityBeforePromptCacheKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &OpenAIGatewayService{}

	bodyA := []byte(`{"prompt_cache_key":"seed-a"}`)
	bodyB := []byte(`{"prompt_cache_key":"seed-b"}`)

	wa := httptest.NewRecorder()
	ca, _ := gin.CreateTestContext(wa)
	ca.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ca.Request.Header.Set("X-Session-Affinity", "sess-affinity-123")

	wb := httptest.NewRecorder()
	cb, _ := gin.CreateTestContext(wb)
	cb.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	cb.Request.Header.Set("X-Session-Affinity", "sess-affinity-123")

	hA := svc.GenerateSessionHash(ca, bodyA)
	hB := svc.GenerateSessionHash(cb, bodyB)

	require.Equal(t, hA, hB)
}
```

- [ ] **Step 2: 跑测试确认当前失败**

Run: `go test ./internal/service -run "GenerateSessionHash_UsesXSessionAffinityBeforePromptCacheKey" -count=1`

Expected: FAIL，因为当前会落到不同的 `prompt_cache_key`。

- [ ] **Step 3: 实现统一 session 信号提取 helper**

```go
type openAISessionSignal struct {
	Source     string
	Value      string
	ParentID   string
	HasSession bool
}

func extractOpenAISessionSignal(c *gin.Context, body []byte) openAISessionSignal {
	if c != nil {
		if v := strings.TrimSpace(c.GetHeader("session_id")); v != "" {
			return openAISessionSignal{Source: "session_id", Value: v, ParentID: strings.TrimSpace(c.GetHeader("x-parent-session-id")), HasSession: true}
		}
		if v := strings.TrimSpace(c.GetHeader("conversation_id")); v != "" {
			return openAISessionSignal{Source: "conversation_id", Value: v, ParentID: strings.TrimSpace(c.GetHeader("x-parent-session-id")), HasSession: true}
		}
		if v := strings.TrimSpace(c.GetHeader("x-session-affinity")); v != "" {
			return openAISessionSignal{Source: "x_session_affinity", Value: v, ParentID: strings.TrimSpace(c.GetHeader("x-parent-session-id")), HasSession: true}
		}
	}
	if len(body) > 0 {
		if v := strings.TrimSpace(gjson.GetBytes(body, "prompt_cache_key").String()); v != "" {
			return openAISessionSignal{Source: "prompt_cache_key", Value: v, HasSession: true}
		}
		if v := deriveOpenAIContentSessionSeed(body); v != "" {
			return openAISessionSignal{Source: "content_fallback", Value: v, HasSession: true}
		}
	}
	return openAISessionSignal{Source: "none"}
}
```

- [ ] **Step 4: 同步修改 sticky hash 与 compat/upstream cache 注入链**

```go
func (s *OpenAIGatewayService) ExtractSessionID(c *gin.Context, body []byte) string {
	return extractOpenAISessionSignal(c, body).Value
}

func (s *OpenAIGatewayService) GenerateSessionHash(c *gin.Context, body []byte) string {
	signal := extractOpenAISessionSignal(c, body)
	if !signal.HasSession {
		return ""
	}
	currentHash, legacyHash := deriveOpenAISessionHashes(signal.Value)
	attachOpenAILegacySessionHashToGin(c, legacyHash)
	return currentHash
}
```

并确保 `/v1/chat/completions` compat cache key 与 `/v1/responses` 直连上游 session/cache 注入链都读这个统一 helper。

- [ ] **Step 5: 增加两条注入链测试**

```go
func TestExtractSessionID_UsesXSessionAffinityBeforePromptCacheKey(t *testing.T) { ... }
func TestBuildUpstreamRequest_UsesXSessionAffinityForOAuthSessionInjection(t *testing.T) { ... }
```

- [ ] **Step 6: 跑通过测试**

Run: `go test ./internal/service -run "GenerateSessionHash_|ExtractSessionID_|BuildUpstreamRequest_UsesXSessionAffinity" -count=1`

Expected: PASS。

- [ ] **Step 7: Commit**

```bash
git add backend/internal/service/openai_gateway_service.go backend/internal/service/openai_gateway_service_test.go
git commit -m "fix(openai): 接入 x-session-affinity 会话信号"
```

### Task 2: 在 handler/service 链中生成 sticky 观测结果并通过 context 传递

**Files:**
- Modify: `backend/internal/service/openai_account_scheduler.go`
- Modify: `backend/internal/service/openai_routing_observability.go`
- Modify: `backend/internal/handler/openai_gateway_handler.go`
- Modify: `backend/internal/handler/openai_chat_completions.go`
- Modify: `backend/internal/handler/openai_gateway_handler_test.go`
- Modify: `backend/internal/service/openai_gateway_service_test.go`

- [ ] **Step 1: 扩展 routing snapshot 结构承载 sticky 观测**

```go
type OpenAIStickyObservability struct {
	SessionSource          string
	SessionHashPresent     bool
	EvalResult             string
	SelectedAccountChanged bool
	ParentSessionPresent   bool
	ParentSessionKey       string
}

type OpenAIRoutingSnapshot struct {
	...
	Sticky *OpenAIStickyObservability
}
```

- [ ] **Step 2: 在调度层生成“首次 sticky 判定结果”与 miss 原因**

```go
type openAIStickyEval struct {
	SessionSource          string
	SessionHashPresent     bool
	EvalResult             string
	SelectedAccountChanged bool
	ParentSessionPresent   bool
	ParentSessionKey       string
	BoundAccountID         int64
}

type OpenAIAccountScheduleRequest struct {
	...
	SessionSource        string
	ParentSessionPresent bool
	ParentSessionKey     string
}

type OpenAIAccountScheduleDecision struct {
	...
	Sticky *openAIStickyEval
}
```

实现约束：

- `sticky_eval_result` 必须由 scheduler / sticky helper 真实生成，不能由 handler 反推。
- `miss_no_binding` / `miss_binding_invalid` / `miss_binding_restricted` / `miss_binding_excluded` / `bypassed_previous_response_id` / `no_session_signal` 都在调度层定案。
- handler 必须先把 `SessionSource` / `ParentSessionPresent` / `ParentSessionKey` 明确传入 `OpenAIAccountScheduleRequest`，scheduler 再据此产出完整的 `scheduleDecision.Sticky`。
- handler 只负责消费 `scheduleDecision.Sticky` 并写入 snapshot/context。

- [ ] **Step 3: 在 handler 里把调度层返回的 sticky 观测挂进 snapshot/context**

```go
snapshot := storeOpenAIRoutingSnapshot(c, service.OpenAIRoutingSnapshotInput{
	TargetGroup:   targetGroup,
	ScheduleLayer: scheduleLayer,
	Account:       account,
	RequestedModel: requestedModel,
	EffectiveModel: effectiveModel,
	Sticky: convertSchedulerStickyEval(scheduleDecision.Sticky),
})
```

要求口径：
- `EvalResult` 记录**首次 sticky 判定结果**
- 后续 failover/重选不覆盖该字段
- `BoundAccountID` 保留“首次 sticky 绑定账号”
- `SelectedAccountChanged` 不在首次判定时拍死，而是在 handler 收到最终成功/失败结果后，用“最终选中账号 vs BoundAccountID”再次回填，保证它反映的是**最终账号变化**而不是首次选号结果

- [ ] **Step 4: 增加 request-level 测试**

```go
func TestStoreOpenAIRoutingSnapshot_PersistsStickyObservability(t *testing.T) {
	...
	require.Equal(t, "x_session_affinity", snap.Sticky.SessionSource)
	require.Equal(t, "hit", snap.Sticky.EvalResult)
}
```

- [ ] **Step 5: 跑测试确认通过**

Run: `go test ./internal/handler ./internal/service -run "StoreOpenAIRoutingSnapshot|StickyObservability|StickySessionHit|SelectAccountWithScheduler" -count=1`

Expected: PASS。

- [ ] **Step 6: Commit**

```bash
git add backend/internal/service/openai_account_scheduler.go backend/internal/service/openai_account_scheduler_test.go backend/internal/service/openai_routing_observability.go backend/internal/handler/openai_gateway_handler.go backend/internal/handler/openai_chat_completions.go backend/internal/handler/openai_gateway_handler_test.go backend/internal/service/openai_gateway_service_test.go
git commit -m "feat(openai): 生成 sticky 观测摘要"
```

### Task 3: success/error 双链持久化 sticky 观测字段

**Files:**
- Modify: `backend/internal/service/openai_gateway_service.go`
- Modify: `backend/internal/service/usage_log.go`
- Modify: `backend/ent/schema/usage_log.go`
- Create: `backend/migrations/092_add_openai_sticky_observability.sql`
- Modify: `backend/internal/repository/usage_log_repo.go`
- Modify: `backend/internal/service/ops_port.go`
- Modify: `backend/internal/repository/ops_repo.go`
- Modify: `backend/internal/handler/ops_error_logger.go`

- [ ] **Step 1: 给 success/error 两条输入结构补字段**

```go
StickySessionSource          *string
StickySessionHashPresent     *bool
StickyEvalResult             *string
StickySelectedAccountChanged *bool
StickyParentSessionPresent   *bool
StickyParentSessionKey       *string
```

- [ ] **Step 2: 为 `usage_logs` 和 `ops_error_logs` 添加 migration**

```sql
ALTER TABLE usage_logs ADD COLUMN IF NOT EXISTS sticky_session_source VARCHAR(32);
ALTER TABLE usage_logs ADD COLUMN IF NOT EXISTS sticky_session_hash_present BOOLEAN;
ALTER TABLE usage_logs ADD COLUMN IF NOT EXISTS sticky_eval_result VARCHAR(64);
ALTER TABLE usage_logs ADD COLUMN IF NOT EXISTS sticky_selected_account_changed BOOLEAN;
ALTER TABLE usage_logs ADD COLUMN IF NOT EXISTS sticky_parent_session_present BOOLEAN;
ALTER TABLE usage_logs ADD COLUMN IF NOT EXISTS sticky_parent_session_key VARCHAR(128);

ALTER TABLE ops_error_logs ADD COLUMN IF NOT EXISTS sticky_session_source VARCHAR(32);
ALTER TABLE ops_error_logs ADD COLUMN IF NOT EXISTS sticky_session_hash_present BOOLEAN;
ALTER TABLE ops_error_logs ADD COLUMN IF NOT EXISTS sticky_eval_result VARCHAR(64);
ALTER TABLE ops_error_logs ADD COLUMN IF NOT EXISTS sticky_selected_account_changed BOOLEAN;
ALTER TABLE ops_error_logs ADD COLUMN IF NOT EXISTS sticky_parent_session_present BOOLEAN;
ALTER TABLE ops_error_logs ADD COLUMN IF NOT EXISTS sticky_parent_session_key VARCHAR(128);
```

- [ ] **Step 3: 扩展 `usage_log_repo.go` 的手写列顺序**

```go
const usageLogSelectColumns = "..., sticky_session_source, sticky_session_hash_present, sticky_eval_result, sticky_selected_account_changed, sticky_parent_session_present, sticky_parent_session_key, ..."
```

同时同步：
- insert SQL
- scan 顺序
- args 顺序

- [ ] **Step 4: 在 success 链写入字段**

```go
usageLog.StickySessionSource = optionalTrimmedStringPtr(snapshot.Sticky.SessionSource)
usageLog.StickySessionHashPresent = optionalBoolPtr(snapshot.Sticky.SessionHashPresent)
usageLog.StickyEvalResult = optionalTrimmedStringPtr(snapshot.Sticky.EvalResult)
usageLog.StickySelectedAccountChanged = optionalBoolPtr(snapshot.Sticky.SelectedAccountChanged)
usageLog.StickyParentSessionPresent = optionalBoolPtr(snapshot.Sticky.ParentSessionPresent)
usageLog.StickyParentSessionKey = optionalTrimmedStringPtr(snapshot.Sticky.ParentSessionKey)
```

并明确修改 `OpenAIGatewayService.buildOpenAIRecordUsageLog` 以及 `/v1/chat/completions` / `/v1/responses` 的 `RecordUsage(...)` 调用，让 `RoutingSnapshot` 真正传进 success 链，而不是只停留在 error/context 里。

- [ ] **Step 5: 在 error 链写入字段**

```go
entry.StickySessionSource = optionalTrimmedStringPtr(snapshot.Sticky.SessionSource)
entry.StickySessionHashPresent = optionalBoolPtr(snapshot.Sticky.SessionHashPresent)
entry.StickyEvalResult = optionalTrimmedStringPtr(snapshot.Sticky.EvalResult)
entry.StickySelectedAccountChanged = optionalBoolPtr(snapshot.Sticky.SelectedAccountChanged)
entry.StickyParentSessionPresent = optionalBoolPtr(snapshot.Sticky.ParentSessionPresent)
entry.StickyParentSessionKey = optionalTrimmedStringPtr(snapshot.Sticky.ParentSessionKey)
```

- [ ] **Step 6: 跑持久化测试**

Run:
- `go test ./internal/repository ./internal/handler -run "UsageLog|OpsError|StickySession" -count=1`

Expected: PASS。

- [ ] **Step 7: Commit**

```bash
git add backend/internal/service/usage_log.go backend/ent/schema/usage_log.go backend/migrations/092_add_openai_sticky_observability.sql backend/internal/repository/usage_log_repo.go backend/internal/service/ops_port.go backend/internal/repository/ops_repo.go backend/internal/handler/ops_error_logger.go
git commit -m "feat(ops): 持久化 sticky session 观测字段"
```

### Task 4: 接通 request details 展示链

**Files:**
- Modify: `backend/internal/service/ops_request_details.go`
- Modify: `backend/internal/repository/ops_repo_request_details.go`
- Modify: `frontend/src/api/admin/ops.ts`
- Modify: `frontend/src/views/admin/ops/components/OpsRequestDetailsModal.vue`

- [ ] **Step 1: 扩展 request details DTO**

```go
StickySessionSource          *string `json:"sticky_session_source,omitempty"`
StickySessionHashPresent     *bool   `json:"sticky_session_hash_present,omitempty"`
StickyEvalResult             *string `json:"sticky_eval_result,omitempty"`
StickySelectedAccountChanged *bool   `json:"sticky_selected_account_changed,omitempty"`
StickyParentSessionPresent   *bool   `json:"sticky_parent_session_present,omitempty"`
StickyParentSessionKey       *string `json:"sticky_parent_session_key,omitempty"`
```

- [ ] **Step 2: 在 success/error 联合 SQL 里都读出这些字段**

```sql
SELECT ..., ul.sticky_session_source, ul.sticky_session_hash_present, ul.sticky_eval_result, ul.sticky_selected_account_changed, ul.sticky_parent_session_present, ul.sticky_parent_session_key FROM usage_logs ul
UNION ALL
SELECT ..., o.sticky_session_source, o.sticky_session_hash_present, o.sticky_eval_result, o.sticky_selected_account_changed, o.sticky_parent_session_present, o.sticky_parent_session_key FROM ops_error_logs o
```

- [ ] **Step 3: 前端 request details 类型同步**

```ts
sticky_session_source?: string
sticky_session_hash_present?: boolean
sticky_eval_result?: string
sticky_selected_account_changed?: boolean
sticky_parent_session_present?: boolean
sticky_parent_session_key?: string
```

- [ ] **Step 4: 在 `OpsRequestDetailsModal.vue` 中增加展示段**

```vue
<div class="grid grid-cols-2 gap-3 text-xs">
  <div><span>{{ t('admin.ops.requestDetails.stickySessionSource') }}</span><span>{{ row.sticky_session_source || '-' }}</span></div>
  <div><span>{{ t('admin.ops.requestDetails.stickyEvalResult') }}</span><span>{{ row.sticky_eval_result || '-' }}</span></div>
  <div><span>{{ t('admin.ops.requestDetails.stickySelectedAccountChanged') }}</span><span>{{ formatBool(row.sticky_selected_account_changed) }}</span></div>
</div>
```

- [ ] **Step 5: 跑 request details 相关验证**

Run:
- `go test ./internal/repository ./internal/handler -run "RequestDetails" -count=1`
- `pnpm typecheck`

Expected: PASS。

- [ ] **Step 6: Commit**

```bash
git add backend/internal/service/ops_request_details.go backend/internal/repository/ops_repo_request_details.go frontend/src/api/admin/ops.ts frontend/src/views/admin/ops/components/OpsRequestDetailsModal.vue
git commit -m "feat(ops): 在请求详情展示 sticky 观测"
```

### Task 5: 新增独立 sticky stats 后端接口

**Files:**
- Modify: `backend/internal/service/ops_port.go`
- Create: `backend/internal/service/ops_openai_sticky_stats.go`
- Create: `backend/internal/repository/ops_repo_openai_sticky_stats.go`
- Modify: `backend/internal/handler/admin/ops_dashboard_handler.go`
- Modify: `backend/internal/server/routes/admin.go`

- [ ] **Step 1: 定义独立 sticky stats 响应结构**

```go
type OpsOpenAIStickyStatsResponse struct {
	TimeRange string `json:"time_range"`
	SessionSourceCountByType  map[string]int64 `json:"session_source_count_by_type"`
	StickyEvalResultCount     map[string]int64 `json:"sticky_eval_result_count"`
	StickyHitRate             float64          `json:"sticky_hit_rate"`
	StickyAccountSwitchRate   float64          `json:"sticky_account_switch_rate"`
}
```

- [ ] **Step 2: 新建 repo SQL，明确 success+error 联合口径**

```sql
WITH combined AS (
  SELECT sticky_session_source, sticky_eval_result, sticky_selected_account_changed
  FROM usage_logs
  WHERE created_at >= $1 AND created_at < $2
  UNION ALL
  SELECT sticky_session_source, sticky_eval_result, sticky_selected_account_changed
  FROM ops_error_logs
  WHERE created_at >= $1 AND created_at < $2
)
SELECT ...
```

把 `sticky_hit_rate` 分母规则直接写进实现，不留给实现者猜：

```sql
COUNT(*) FILTER (
  WHERE sticky_eval_result IN (
    'hit',
    'miss_no_binding',
    'miss_binding_invalid',
    'miss_binding_restricted',
    'miss_binding_excluded'
  )
) AS sticky_evaluated_total
```

```go
resp.StickyHitRate = percent(hitCount, stickyEvaluatedTotal)
```

`bypassed_previous_response_id` 与 `no_session_signal` 只进入 `sticky_eval_result_count`，**不进入 hit rate 分母**。

`sticky_account_switch_rate` 的分母同样只统计真正进入 sticky 判定的请求（与 `sticky_hit_rate` 共用 `sticky_evaluated_total`），不把 `bypassed_previous_response_id` / `no_session_signal` 混进去。

- [ ] **Step 3: 增加 handler 与路由**

先补接口，再补实现：

```go
// in ops_port.go
GetOpenAIStickyStats(ctx context.Context, input *OpsOpenAIStickyStatsInput) (*OpsOpenAIStickyStatsResponse, error)
```

并同步更新 `backend/internal/service/ops_repo_mock_test.go` 里的 `OpsRepository` mock/stub，确保接口扩展后测试能编译。

然后补 service / handler / route：

```go
ops.GET("/dashboard/openai-sticky", h.Admin.Ops.GetOpenAIStickyStats)
```

- [ ] **Step 4: 跑后端接口验证**

Run:
- `go test ./internal/handler ./internal/repository ./internal/service -run "OpenAIStickyStats" -count=1`

Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add backend/internal/service/ops_port.go backend/internal/service/ops_repo_mock_test.go backend/internal/service/ops_openai_sticky_stats.go backend/internal/repository/ops_repo_openai_sticky_stats.go backend/internal/handler/admin/ops_dashboard_handler.go backend/internal/server/routes/admin.go
git commit -m "feat(ops): 增加 OpenAI sticky 聚合接口"
```

### Task 6: 前端 sticky 卡片与总览挂载

**Files:**
- Modify: `frontend/src/api/admin/ops.ts`
- Create: `frontend/src/views/admin/ops/components/OpsOpenAIStickyCard.vue`
- Modify: `frontend/src/views/admin/ops/OpsDashboard.vue`
- Modify: `frontend/src/i18n/locales/en.ts`
- Modify: `frontend/src/i18n/locales/zh.ts`

- [ ] **Step 1: 扩展前端 API 类型与方法**

```ts
export interface OpsOpenAIStickyStatsResponse {
  time_range: string
  session_source_count_by_type: Record<string, number>
  sticky_eval_result_count: Record<string, number>
  sticky_hit_rate: number
  sticky_account_switch_rate: number
}

getOpenAIStickyStats(params: Record<string, any>) {
  return get<OpsOpenAIStickyStatsResponse>('/admin/ops/dashboard/openai-sticky', { params })
}
```

- [ ] **Step 2: 创建 `OpsOpenAIStickyCard.vue`**

```vue
<section class="card p-4 md:p-5">
  <h3>{{ t('admin.ops.openaiSticky.title') }}</h3>
  <p>{{ t('admin.ops.openaiSticky.subtitle') }}</p>
  <!-- 显示：session source 分布、sticky hit rate、eval result 分布、account switch rate -->
</section>
```

- [ ] **Step 3: 在 Dashboard 中挂载**

```vue
<OpsOpenAIRoutingCard ... />
<OpsOpenAIRetryCard ... />
<OpsOpenAIStickyCard ... />
```

- [ ] **Step 4: 增加文案**

```ts
openaiSticky: {
  title: 'OpenAI Session Affinity & Sticky',
  subtitle: 'Session source, sticky evaluation and account switching across OpenAI HTTP requests.',
  stickyHitRate: 'Sticky Hit Rate',
  stickyAccountSwitchRate: 'Account Switch Rate',
  sessionSource: 'Session Source',
  stickyEvalResult: 'Sticky Evaluation Result'
}
```

- [ ] **Step 5: 跑前端验证**

Run: `pnpm typecheck`

Expected: PASS。

- [ ] **Step 6: Commit**

```bash
git add frontend/src/api/admin/ops.ts frontend/src/views/admin/ops/components/OpsOpenAIStickyCard.vue frontend/src/views/admin/ops/OpsDashboard.vue frontend/src/i18n/locales/en.ts frontend/src/i18n/locales/zh.ts
git commit -m "feat(ops): 增加 OpenAI sticky 观测卡片"
```

### Task 7: 收尾验证与进度记录

**Files:**
- Modify: `backend/docs/superpowers/reports/2026-04-08-sub2api-merged-prep.md`

- [ ] **Step 1: 更新进度报告**

```md
- `x-session-affinity` 已接入 OpenAI HTTP 主链 sticky/session hash 与 upstream cache/session 注入
- sticky/session 观测已进入 success/error 双链
- request details 与 ops 面板已可观察 session 来源、sticky 结果和账号切换率
```

- [ ] **Step 2: 运行最终全量验证**

Run:
- `go test ./internal/handler ./internal/repository ./internal/server/... -count=1`
- `go test -tags unit ./internal/service ./internal/pkg/apicompat -count=1`
- `go build ./cmd/server`
- `pnpm typecheck`

Expected: 全部 PASS。

- [ ] **Step 3: 最终提交**

```bash
git add backend/docs/superpowers/reports/2026-04-08-sub2api-merged-prep.md
git commit -m "feat(openai): 接入 session affinity 并补齐 sticky 观测"
```
