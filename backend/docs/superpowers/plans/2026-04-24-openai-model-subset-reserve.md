# OpenAI Model-Subset Reserve Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让 OpenAI exhausted / reserve 从“账号整体静态分桶”升级为“按模型可达子集预计算出的派生投影”，并保持 reserve 仍然只是 exhausted overflow 子组。

**Architecture:** 先新增一层 OpenAI 模型子集投影：基于 `scheduler bucket + canonical routing model`、有限的 canonical model catalog、以及账号能力快照，离线重算每个 bucket 的 exhausted 基础池和 reserve overflow 池，再把局部 reserve 候选提升成账号级一致 reserve 身份。请求热路径不再 live 推导 reserve，只消费 snapshot/cache 中已发布的投影结果；账号状态变化时通过现有 outbox/snapshot 链整体重算并原子切换版本。

**Tech Stack:** Go, existing OpenAI gateway/scheduler stack, scheduler snapshot service, Redis scheduler cache, PostgreSQL account repository, existing OpenAI compat normalization and reserve tests.

---

## File Map

- Create: `backend/internal/service/openai_model_subset_projection.go`
  - 定义 canonical model catalog、能力快照输入、模型子集局部计算、账号级 reserve 提升、投影结果结构。
- Create: `backend/internal/service/openai_model_subset_projection_test.go`
  - 纯单元测试：新模型双账号子集、未知模型保守排除、`3账号x2模型` 不对称矩阵、`exhausted=0 => 100%`、全模型一致 reserve。
- Modify: `backend/internal/service/account.go`
  - 补充投影专用的“保守模型可达判断” helper，不能再沿用 `len(mapping)==0 => true` 作为投影默认语义。
- Modify: `backend/internal/service/openai_compat_model.go`
  - 暴露投影层复用的 canonical model 规范化 helper，统一 `-Sys` / compat / Codex 归一化。
- Modify: `backend/internal/service/openai_gateway_service.go`
  - 将 `listSchedulableAccounts(...)`、`listOpenAIExhaustedWithReserveOverlay(...)`、sticky / previous_response / failover recheck 等请求态消费者改成只读 projection。
- Modify: `backend/internal/service/openai_account_scheduler.go`
  - 选择阶段不再用 live account list 推 reserve；只消费 projection 给出的 exhausted/reserve 参与者，并保持 active/any reserve exclusion 语义。
- Modify: `backend/internal/service/openai_ws_account_sticky_test.go`
  - 增加 projection_version / reserve affinity / exhausted 回落矩阵验证。
- Modify: `backend/internal/service/openai_account_scheduler_test.go`
  - 锁住 reserve 阈值矩阵、active/any 不误选 reserve、unknown-model 保守排除、`-Sys` 与 canonical model 共用同一投影键。
- Modify: `backend/internal/service/openai_gateway_service_test.go`
  - 验证 list/selection/fallback 读取 projection，不再在热路径 live 推导 reserve。
- Modify: `backend/internal/service/scheduler_cache.go`
  - 扩展 `SchedulerCache` 接口，支持按 bucket/version 读写 projection payload 与版本字段。
- Modify: `backend/internal/repository/scheduler_cache.go`
  - 在 Redis snapshot 发布中存取 projection JSON，并保持 bucket 级原子切换。
- Modify: `backend/internal/service/scheduler_snapshot_service.go`
  - 在 rebuildBucket / fallback 路径里生成并发布 projection，确保 request 侧只读 cache/snapshot。
- Modify: `backend/internal/service/scheduler_snapshot_hydration_test.go`
  - 验证 projection 字段能随 snapshot 一起 hydration。
- Modify: `backend/internal/repository/account_repo.go`
  - 把影响 projection 的 `UpdateCredentials` / `UpdateExtra` 字段从 scheduler-neutral 旁路收口到 projection refresh 触发链。
- Modify: `backend/internal/repository/account_repo_integration_test.go`
  - 验证 `UpdateCredentials`、Codex quota snapshot、能力字段变化会触发 outbox / projection rebuild。
- Create: `backend/internal/service/openai_model_subset_projection_integration_test.go`
  - 验证 projection_version 增长、atomic publish、请求只消费新版本 projection。

## Task 1: 建立 canonical model catalog 与保守能力快照

**Files:**
- Create: `backend/internal/service/openai_model_subset_projection.go`
- Create: `backend/internal/service/openai_model_subset_projection_test.go`
- Modify: `backend/internal/service/account.go`
- Modify: `backend/internal/service/openai_compat_model.go`

- [ ] **Step 1: 先写 catalog / 能力快照红灯测试**

```go
func TestBuildOpenAICanonicalModelCatalog_ExpandsOnlyFiniteSources(t *testing.T) {
	accounts := []Account{
		{Credentials: map[string]any{"model_mapping": map[string]any{"gpt-5.4": "gpt-5.4", "gpt-5*": "gpt-5.4"}}},
	}
	catalog := BuildOpenAICanonicalModelCatalog(accounts, []string{"gpt-5.4", "gpt-5.5"}, []string{"gpt-5.4"})
	require.Contains(t, catalog, "gpt-5.4")
	require.Contains(t, catalog, "gpt-5.5")
	require.NotContains(t, catalog, "gpt-unknown-live")
}

func TestProjectionModelReachability_UnknownModelNeedsExplicitCapability(t *testing.T) {
	account := Account{Credentials: map[string]any{}, Extra: map[string]any{}}
	require.False(t, accountSupportsProjectionModel(account, "gpt-5.6"))
}

func TestNormalizeOpenAIProjectionModelKey_ReusesCompatNormalization(t *testing.T) {
	require.Equal(t, "gpt-5.4", NormalizeOpenAIProjectionModelKey("gpt-5.4-Sys"))
}
```

- [ ] **Step 2: 跑测试确认 RED**

Run: `C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe test -tags unit ./internal/service -run "TestBuildOpenAICanonicalModelCatalog_.*|TestProjectionModelReachability_.*|TestNormalizeOpenAIProjectionModelKey_.*" -count=1`

Expected: FAIL，原因是 catalog / projection helper 尚不存在，且当前 `IsModelSupported` 仍把无 mapping 视为支持全部模型。

- [ ] **Step 3: 写最小 catalog 与 projection model key helper**

```go
func NormalizeOpenAIProjectionModelKey(model string) string {
	trimmed := strings.TrimSpace(model)
	trimmed = strings.TrimSuffix(trimmed, "-Sys")
	return NormalizeOpenAICompatRequestedModel(normalizeCodexModel(trimmed))
}

type OpenAIModelCapabilitySnapshot struct {
	ExplicitModels map[string]struct{}
	WildcardRules  []string
	DefaultAllow   bool
}

func BuildOpenAICanonicalModelCatalog(accounts []Account, explicitCapabilityModels []string, configuredModels []string) []string {
	// 只从有限目录来源聚合 canonical model keys
}
```

- [ ] **Step 4: 给 `Account` 增加投影专用保守判断 helper**

```go
func (a *Account) SupportsProjectionModel(model string, snapshot OpenAIModelCapabilitySnapshot) bool {
	canonical := NormalizeOpenAIProjectionModelKey(model)
	if canonical == "" {
		return false
	}
	if _, ok := snapshot.ExplicitModels[canonical]; ok {
		return true
	}
	return false
}
```

- [ ] **Step 5: 跑测试转 GREEN**

Run: `C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe test -tags unit ./internal/service -run "TestBuildOpenAICanonicalModelCatalog_.*|TestProjectionModelReachability_.*|TestNormalizeOpenAIProjectionModelKey_.*" -count=1`

Expected: PASS。

- [ ] **Step 6: 提交这一小步**

```bash
git add backend/internal/service/openai_model_subset_projection.go backend/internal/service/openai_model_subset_projection_test.go backend/internal/service/account.go backend/internal/service/openai_compat_model.go
git commit -m "fix(openai): 增加模型子集投影基础能力"
```

## Task 2: 实现模型子集 reserve 派生与账号级一致提升

**Files:**
- Modify: `backend/internal/service/openai_model_subset_projection.go`
- Modify: `backend/internal/service/openai_model_subset_projection_test.go`

- [ ] **Step 1: 先写局部子集 -> 全模型一致 reserve 红灯测试**

```go
func TestBuildOpenAIModelSubsetProjection_NewModelTwoAccountsPromotesReserve(t *testing.T) {
	accounts := []Account{
		newOpenAIActiveWithCapability(1, []string{"gpt-5.6"}),
		newOpenAIActiveWithCapability(2, []string{"gpt-5.6"}),
	}
	projection := BuildOpenAIModelSubsetProjection(SchedulerBucket{GroupID: 2, Platform: PlatformOpenAI, Mode: SchedulerModeSingle}, accounts, []string{"gpt-5.6"})
	view := projection.ViewForModel("gpt-5.6")
	require.NotEmpty(t, view.ReserveOverflowIDs)
}

func TestBuildOpenAIModelSubsetProjection_AsymmetricMatrixLiftsReserveAcrossSupportedModels(t *testing.T) {
	// 3 账号 x 2 模型：账号 A 支持 gpt-5.6 和 gpt-5.4，账号 B 只支持 gpt-5.6，账号 C 只支持 gpt-5.4
	// 若 A 在 gpt-5.6 子集中被提升为 reserve，则在 gpt-5.4 里也保持 reserve。
}

func TestBuildOpenAIModelSubsetProjection_ExhaustedEmptyMeansOneHundredPercent(t *testing.T) {
	// exhausted 子集为空时，reserve overflow 规则仍按 100% 占用触发
}
```

- [ ] **Step 2: 跑测试确认 RED**

Run: `C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe test -tags unit ./internal/service -run "TestBuildOpenAIModelSubsetProjection_.*" -count=1`

Expected: FAIL，原因是 projection builder 还没实现上述语义。

- [ ] **Step 3: 写 projection 数据结构与局部计算**

```go
type OpenAIModelRoleView struct {
	CanonicalModel     string
	ExhaustedBaseIDs   []int64
	ReserveOverflowIDs []int64
}

type OpenAIModelSubsetProjection struct {
	Bucket            SchedulerBucket
	ProjectionVersion int64
	BuiltAt           time.Time
	AccountReserveIDs map[int64]struct{}
	Models            map[string]OpenAIModelRoleView
}
```

- [ ] **Step 4: 实现账号级一致 reserve 提升与不变量**

```go
func liftModelSubsetReserveIdentities(local map[string]OpenAIModelRoleView, supportedModels map[int64][]string) map[string]OpenAIModelRoleView {
	// 仅提升已在某模型子集里属于 reserve overflow candidate 的账号
	// exhausted base 与 reserve overflow 在同一模型子集互斥
}
```

- [ ] **Step 5: 跑测试转 GREEN**

Run: `C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe test -tags unit ./internal/service -run "TestBuildOpenAIModelSubsetProjection_.*" -count=1`

Expected: PASS。

- [ ] **Step 6: 提交这一小步**

```bash
git add backend/internal/service/openai_model_subset_projection.go backend/internal/service/openai_model_subset_projection_test.go
git commit -m "fix(openai): 实现模型子集 reserve 派生投影"
```

## Task 3: 把 projection 接入 scheduler snapshot / cache 发布

**Files:**
- Modify: `backend/internal/service/scheduler_cache.go`
- Modify: `backend/internal/repository/scheduler_cache.go`
- Modify: `backend/internal/service/scheduler_snapshot_service.go`
- Modify: `backend/internal/service/scheduler_snapshot_hydration_test.go`
- Create: `backend/internal/service/openai_model_subset_projection_integration_test.go`

- [ ] **Step 1: 先写 snapshot/hydration 红灯测试**

```go
func TestSchedulerSnapshotHydration_PreservesOpenAIProjectionFields(t *testing.T) {
	cache := &snapshotHydrationCache{ /* snapshot + projection */ }
	svc := &OpenAIGatewayService{schedulerSnapshot: NewSchedulerSnapshotService(cache, nil, nil, nil, nil)}
	projection, ok := svc.GetOpenAIModelSubsetProjectionForTest(SchedulerBucket{GroupID: 2, Platform: PlatformOpenAI, Mode: SchedulerModeSingle})
	require.True(t, ok)
	require.Equal(t, int64(7), projection.ProjectionVersion)
}
```

- [ ] **Step 2: 跑测试确认 RED**

Run: `C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe test -tags unit ./internal/service -run "TestSchedulerSnapshotHydration_PreservesOpenAIProjectionFields|TestOpenAIModelSubsetProjectionIntegration_.*" -count=1`

Expected: FAIL，原因是 cache/snapshot 还不会读写 projection。

- [ ] **Step 3: 扩展 `SchedulerCache` 接口和 Redis 实现**

```go
type SchedulerCache interface {
	GetSnapshot(ctx context.Context, bucket SchedulerBucket) ([]*Account, bool, error)
	SetSnapshot(ctx context.Context, bucket SchedulerBucket, accounts []Account) error
	GetOpenAIProjection(ctx context.Context, bucket SchedulerBucket) (*OpenAIModelSubsetProjection, bool, error)
	SetOpenAIProjection(ctx context.Context, bucket SchedulerBucket, projection *OpenAIModelSubsetProjection) error
}
```

- [ ] **Step 4: 在 `rebuildBucket(...)` 中生成并原子发布 projection**

```go
if bucket.Platform == PlatformOpenAI {
	projection := BuildOpenAIModelSubsetProjection(bucket, accounts, catalog)
	if err := s.cache.SetOpenAIProjection(ctx, bucket, projection); err != nil { ... }
}
```

- [ ] **Step 5: 补 atomic publish/version 用例并转 GREEN**

Run: `C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe test -tags unit ./internal/service -run "TestSchedulerSnapshotHydration_PreservesOpenAIProjectionFields|TestOpenAIModelSubsetProjectionIntegration_.*" -count=1`

Expected: PASS。

- [ ] **Step 6: 提交这一小步**

```bash
git add backend/internal/service/scheduler_cache.go backend/internal/repository/scheduler_cache.go backend/internal/service/scheduler_snapshot_service.go backend/internal/service/scheduler_snapshot_hydration_test.go backend/internal/service/openai_model_subset_projection_integration_test.go
git commit -m "fix(openai): 将模型子集投影接入快照发布"
```

## Task 4: 让请求热路径只消费 projection 结果

**Files:**
- Modify: `backend/internal/service/openai_gateway_service.go`
- Modify: `backend/internal/service/openai_account_scheduler.go`
- Modify: `backend/internal/service/openai_gateway_service_test.go`
- Modify: `backend/internal/service/openai_account_scheduler_test.go`
- Modify: `backend/internal/service/openai_ws_account_sticky_test.go`

- [ ] **Step 1: 先写请求态消费红灯测试**

```go
func TestOpenAIGatewayService_ListOpenAIExhaustedWithReserveOverlay_UsesProjectionInsteadOfLiveBuckets(t *testing.T) {
	// 构造 projection 中 gpt-5.6 只有 2 个账号，其中 1 个是 reserve
	// 即使 live account list 里全局 exhausted/reserve 为空，也应返回 projection 给出的 exhausted/reserve 结果
}

func TestSelectByLoadBalance_TargetGroupAnyStillRejectsOverlayReserveFromProjection(t *testing.T) {
	// active/any 路径继续拒绝 overlay reserve
}

func TestPreviousResponseReserveBindingReadsSameProjectionKeyAsSysModel(t *testing.T) {
	// gpt-5.4-Sys 与 gpt-5.4 命中同一 projection key，但 routing target group 仍是 exhausted
}
```

- [ ] **Step 2: 跑测试确认 RED**

Run: `C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe test -tags unit ./internal/service -run "TestOpenAIGatewayService_ListOpenAIExhaustedWithReserveOverlay_UsesProjectionInsteadOfLiveBuckets|TestSelectByLoadBalance_TargetGroupAnyStillRejectsOverlayReserveFromProjection|TestPreviousResponseReserveBindingReadsSameProjectionKeyAsSysModel" -count=1`

Expected: FAIL，原因是当前热路径还在 live 算 reserve。

- [ ] **Step 3: 在 gateway/scheduler 中改为按 projection 取视图**

```go
func (s *OpenAIGatewayService) getOpenAIProjectionView(ctx context.Context, groupID *int64, requestedModel string) (*OpenAIModelRoleView, *OpenAIModelSubsetProjection, error) {
	bucket := s.schedulerSnapshot.bucketFor(groupID, PlatformOpenAI, SchedulerModeSingle)
	projection, _, err := s.schedulerSnapshot.GetOpenAIProjection(ctx, bucket)
	...
}
```

- [ ] **Step 4: 保留现有 guardrail 与 sticky/previous_response 语义**

```go
// active/any 仍排斥 overlay reserve
// reserve affinity 只对 exhausted 请求有效
// old binding projection_version 不一致时失效或重绑
```

- [ ] **Step 5: 跑相关测试转 GREEN**

Run: `C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe test -tags unit ./internal/service -run "TestOpenAIGatewayService_ListOpenAIExhaustedWithReserveOverlay_UsesProjectionInsteadOfLiveBuckets|TestSelectByLoadBalance_Exhausted.*Reserve|TestSelectByLoadBalance_TargetGroupAnyNeverSelectsReserve|TestSelectByLoadBalance_TargetGroupActiveNeverSelectsReserve|TestPreviousResponseReserveBindingStillMatchesExhaustedClass" -count=1`

Expected: PASS。

- [ ] **Step 6: 提交这一小步**

```bash
git add backend/internal/service/openai_gateway_service.go backend/internal/service/openai_account_scheduler.go backend/internal/service/openai_gateway_service_test.go backend/internal/service/openai_account_scheduler_test.go backend/internal/service/openai_ws_account_sticky_test.go
git commit -m "fix(openai): 让请求主链消费模型子集投影"
```

## Task 5: 把真实写路径纳入 projection refresh 并做整体验证

**Files:**
- Modify: `backend/internal/repository/account_repo.go`
- Modify: `backend/internal/repository/account_repo_integration_test.go`
- Modify: `backend/internal/service/openai_model_subset_projection_integration_test.go`
- Modify: `backend/internal/service/openai_model_subset_projection_test.go`
- Modify: `backend/internal/service/openai_gateway_service_test.go`

- [ ] **Step 1: 先写真实触发链红灯测试**

```go
func TestUpdateCredentials_TriggersProjectionRefresh(t *testing.T) {}
func TestUpdateExtra_CodexSnapshotFieldsTriggerProjectionRefresh(t *testing.T) {}
func TestUnknownModel_IsConservativelyExcludedUntilCapabilitySnapshotRefresh(t *testing.T) {}
```

- [ ] **Step 2: 跑测试确认 RED**

Run: `C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe test ./internal/repository -run "TestUpdateCredentials_TriggersProjectionRefresh|TestUpdateExtra_CodexSnapshotFieldsTriggerProjectionRefresh" -count=1`

Expected: FAIL，原因是这些路径目前仍是 scheduler-neutral 或只做单账号 sync。

- [ ] **Step 3: 最小收口真实写路径**

```go
func projectionRelevantExtraUpdate(updates map[string]any) bool {
	for key := range updates {
		switch key {
		case "codex_7d_used_percent", "codex_primary_used_percent", "model_mapping", "openai_capability_models":
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: 跑完整相关验证**

Run:

```bash
C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe test ./internal/repository -count=1
C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe test -tags unit ./internal/service -run "TestBuildOpenAIModelSubsetProjection_.*|TestOpenAIGatewayService_ListOpenAIExhaustedWithReserveOverlay_UsesProjectionInsteadOfLiveBuckets|TestSelectByLoadBalance_Exhausted.*Reserve|TestPreviousResponseReserveBindingStillMatchesExhaustedClass" -count=1
C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe build ./cmd/server
git diff --check
```

Expected: 全部通过。

- [ ] **Step 5: 记录发版演练清单并提交最终实现**

```text
1. 先 pnpm install / pnpm build（如 worktree 缺 dist）
2. GOOS=linux GOARCH=amd64 CGO_ENABLED=0 构建 embed 二进制
3. 上传到 /tmp
4. 127.0.0.1:18081 smoke，至少验证 /health
5. 如条件允许，做一条真实新模型 exhausted-class 请求链路：
   - 初始无投影命中 -> 状态变化/能力刷新 -> projection_version 增长 -> exhausted-class 命中新的 reserve overflow 账号
6. 提升正式版本并验证 /health
```

- [ ] **Step 6: 提交最终实现**

```bash
git add backend/internal/repository/account_repo.go backend/internal/repository/account_repo_integration_test.go backend/internal/service/openai_model_subset_projection_integration_test.go backend/internal/service/openai_model_subset_projection_test.go backend/internal/service/openai_gateway_service_test.go
git commit -m "test(openai): 补齐模型子集 reserve 回归覆盖"
```
