# OpenAI Model-Subset Reserve Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让 OpenAI exhausted / reserve 从“账号整体静态分桶”升级为“按模型可达子集预计算出的派生投影”，并保持 reserve 仍然只是 exhausted overflow 子组。

**Architecture:** 先新增一层 OpenAI 模型子集投影：基于 `scheduler bucket + canonical routing model`、有限的 canonical model catalog、以及账号能力快照，离线重算每个 bucket 的 exhausted 基础池和 reserve overflow 池，再把局部 reserve 候选提升成账号级一致 reserve 身份。请求热路径不再 live 推导 reserve，只消费 snapshot/cache 中已发布的投影结果；账号状态变化时通过现有 outbox/snapshot 链整体重算并原子切换版本。

**Tech Stack:** Go, existing OpenAI gateway/scheduler stack, scheduler snapshot service, Redis scheduler cache, PostgreSQL account repository, existing OpenAI compat normalization and reserve tests.

---

## File Map

- Create: `backend/internal/service/openai_model_subset_projection.go`
  - 定义 canonical model catalog、能力快照 contract、`OpenAIProjectionInputs`、模型子集局部计算、账号级 reserve 提升、投影结果结构。
- Create: `backend/internal/service/openai_model_subset_projection_test.go`
  - 纯单元测试：未知模型保守排除、wildcard 离线展开边界、`3账号x2模型` 不对称矩阵、reserve 阈值矩阵、`exhausted=0 => 100%`、全模型一致 reserve。
- Modify: `backend/internal/service/account.go`
  - 补充投影专用的“保守模型可达判断” helper，不能再沿用 `len(mapping)==0 => true` 作为投影默认语义。
- Modify: `backend/internal/service/openai_compat_model.go`
  - 暴露投影层复用的 canonical model 规范化 helper，统一 `-Sys` / compat / Codex 归一化。
- Modify: `backend/internal/service/openai_gateway_service.go`
  - 将 `listSchedulableAccounts(...)`、`listOpenAIExhaustedWithReserveOverlay(...)`、failover recheck 等请求态消费者改成只读 projection bundle。
- Modify: `backend/internal/service/openai_account_scheduler.go`
  - 选择阶段不再用 live account list 推 reserve；只消费 projection 给出的 exhausted/reserve 参与者，并保持 active/any reserve exclusion 语义。
- Modify: `backend/internal/service/openai_ws_forwarder.go`
  - `previous_response_id` / continuation 路径只读 projection，并在 projection_version 变化时失效或重绑旧 reserve anchor。
- Modify: `backend/internal/service/openai_sticky_compat.go`
  - sticky / previous_response 绑定校验增加 projection_version / canonical model key 一致性约束。
- Modify: `backend/internal/service/openai_ws_state_store.go`
  - 如果 response affinity 持久化结构需要携带 projection_version / canonical model key，则在此扩展。
- Modify: `backend/internal/service/openai_routing_observability.go`
  - request 级 routing snapshot / 日志带出 `projection_version`、`projection_built_at`、canonical model key。
- Modify: `backend/internal/service/openai_ws_account_sticky_test.go`
  - 增加 projection_version / reserve affinity / exhausted 回落矩阵验证。
- Modify: `backend/internal/service/openai_account_scheduler_test.go`
  - 锁住 reserve 阈值矩阵、active/any 不误选 reserve、unknown-model 保守排除、`-Sys` 与 canonical model 共用同一投影键。
- Modify: `backend/internal/service/openai_gateway_service_test.go`
  - 验证 list/selection/fallback 读取 projection bundle，不再在热路径 live 推导 reserve；projection miss 时 fail-closed。
- Modify: `backend/internal/service/scheduler_cache.go`
  - 扩展 `SchedulerCache` 接口，按 scheduler bucket 读写单一 `accounts + projection + version + built_at` bundle。
- Modify: `backend/internal/repository/scheduler_cache.go`
  - 在 Redis snapshot 发布中存取 bundle JSON，并保持 scheduler bucket 级原子切换与单一 active 指针。
- Modify: `backend/internal/service/scheduler_snapshot_service.go`
  - 在 rebuildBucket / fallback 路径里装配 `OpenAIProjectionInputs`、生成 bundle 并发布；projection miss/snapshot miss 时 fail-closed 或同步重建，但不得回退到 live reserve 推导。
- Modify: `backend/internal/service/scheduler_snapshot_hydration_test.go`
  - 验证 bundle 里的 projection/version/built_at 字段能随 snapshot 一起 hydration。
- Modify: `backend/internal/repository/account_repo.go`
  - 把影响 projection 的真实写路径（`UpdateCredentials`、相关 `UpdateExtra`、temp-unsched/rate-limit/schedulable 变化）收口到 projection refresh 触发链。
- Modify: `backend/internal/repository/account_repo_integration_test.go`
  - 验证真实写路径矩阵会触发 outbox / projection rebuild。
- Create: `backend/internal/service/openai_model_subset_projection_integration_test.go`
  - 验证 projection_version 增长、atomic publish、unknown-model 保守排除与刷新闭环、请求只消费新版本 projection。

## Task 1: 冻结 canonical model catalog、capability snapshot 与 projection key 合同

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

func TestProjectionModelReachability_MissingMappingRejectsUnknownModel(t *testing.T) {
	account := Account{Credentials: map[string]any{}, Extra: map[string]any{}}
	require.False(t, accountSupportsProjectionModel(account, "gpt-5.unknown"))
}

func TestProjectionModelReachability_EmptyMappingRejectsUnknownModel(t *testing.T) {
	account := Account{Credentials: map[string]any{"model_mapping": map[string]any{}}, Extra: map[string]any{}}
	require.False(t, accountSupportsProjectionModel(account, "gpt-5.unknown"))
}

func TestProjectionModelReachability_UnknownModelRejectsNoMappingAndWildcardByDefault(t *testing.T) {
	account := Account{
		Credentials: map[string]any{"model_mapping": map[string]any{"gpt-*": "gpt-5.4"}},
		Extra:       map[string]any{},
	}
	require.False(t, accountSupportsProjectionModel(account, "gpt-5.unknown"))
}

func TestProjectionModelReachability_ExplicitCapabilityOverridesConservativeDefault(t *testing.T) {
	account := Account{Extra: map[string]any{"openai_capability_explicit_models": []string{"gpt-5.6"}}}
	require.True(t, accountSupportsProjectionModel(account, "gpt-5.6"))
}

func TestNormalizeOpenAIProjectionModelKey_ReusesCompatNormalization(t *testing.T) {
	require.Equal(t, "gpt-5.4", NormalizeOpenAIProjectionModelKey("gpt-5.4-Sys"))
}
```

- [ ] **Step 2: 跑测试确认 RED**

Run: `C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe test -tags unit ./internal/service -run "TestBuildOpenAICanonicalModelCatalog_.*|TestProjectionModelReachability_.*|TestNormalizeOpenAIProjectionModelKey_.*" -count=1`

Expected: FAIL，原因是 catalog / projection helper 尚不存在，且当前 `IsModelSupported` 仍把无 mapping 视为支持全部模型，wildcard 也还没有被限制为只能离线扩展 catalog 内模型。

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
	// 只从有限来源聚合 canonical model：
	// 1. capability snapshot explicit models
	// 2. model_mapping 的 canonical key/value
	// 3. group 显式 default/forced models（例如 Group.DefaultMappedModel、OpenAI messages dispatch 里显式配置的模型）
	// 4. channel 侧若存在显式 default/forced source，也只允许从这些显式字段进入 catalog
	// 不允许把 wildcard/default-allow 在请求热路径里临时扩成无限集合
}
```

- [ ] **Step 4: 给 `Account` 增加投影专用保守判断 helper**

```go
func (a *Account) SupportsProjectionModel(model string, snapshot OpenAIModelCapabilitySnapshot, catalog map[string]struct{}) bool {
	canonical := NormalizeOpenAIProjectionModelKey(model)
	if canonical == "" {
		return false
	}
	if _, ok := catalog[canonical]; !ok {
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

Expected: PASS，并且明确锁住：未知模型在 capability snapshot 正向证明前必须被保守排除，wildcard/default-allow 不能绕过 finite catalog 合同。

- [ ] **Step 6: 提交这一小步**

```bash
git add backend/internal/service/openai_model_subset_projection.go backend/internal/service/openai_model_subset_projection_test.go backend/internal/service/account.go backend/internal/service/openai_compat_model.go
git commit -m "fix(openai): 增加模型子集投影基础能力"
```

## Task 2: 先装配 projection 输入全集，再做模型子集 reserve 派生

**Files:**
- Modify: `backend/internal/service/openai_model_subset_projection.go`
- Modify: `backend/internal/service/openai_model_subset_projection_test.go`
- Modify: `backend/internal/service/scheduler_snapshot_service.go`
- Modify: `backend/internal/service/openai_gateway_service_test.go`

- [ ] **Step 1: 先写 projection 输入全集与派生红灯测试**

```go
func TestLoadOpenAIProjectionInputs_PreservesBroadSourceExhaustedMembers(t *testing.T) {
	// schedulable snapshot 里没有 exhausted 账号，但 broad-source exhausted merge 能补回来
	// 断言 projection 输入全集里仍包含 exhausted base 所需账号
}

func TestBuildOpenAIModelSubsetProjection_NewModelTwoAccountsPromotesReserve(t *testing.T) {
	inputs := &OpenAIProjectionInputs{
		Bucket:           SchedulerBucket{GroupID: 2, Platform: PlatformOpenAI, Mode: SchedulerModeSingle},
		CanonicalCatalog: []string{"gpt-5.6"},
	}
	accounts := []Account{
		newOpenAIActiveWithCapability(1, []string{"gpt-5.6"}),
		newOpenAIActiveWithCapability(2, []string{"gpt-5.6"}),
	}
	inputs.AccountsAll = accounts
	projection := BuildOpenAIModelSubsetProjection(inputs)
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

func TestBuildOpenAIModelSubsetProjection_ReserveThresholdMatrix(t *testing.T) {
	// 显式锁住：overflow>60%、<=60%、exhausted=0=>100%、reserve co-participation、active/any never select reserve
}
```

- [ ] **Step 2: 跑测试确认 RED**

Run: `C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe test -tags unit ./internal/service -run "TestLoadOpenAIProjectionInputs_.*|TestBuildOpenAIModelSubsetProjection_.*" -count=1`

Expected: FAIL，原因是 projection builder 还没实现上述语义，而且当前 projection 输入还没有保留 broad-source exhausted 账号全集。

- [ ] **Step 3: 定义 `OpenAIProjectionInputs` 与统一装配入口**

```go
type OpenAIProjectionInputs struct {
	Bucket            SchedulerBucket
	CanonicalCatalog  []string
	AccountsAll       []Account
	ExhaustedBroadIDs map[int64]struct{}
	CapabilityByID    map[int64]OpenAIModelCapabilitySnapshot
}

func (s *SchedulerSnapshotService) loadOpenAIProjectionInputs(ctx context.Context, bucket SchedulerBucket) (*OpenAIProjectionInputs, error) {
	// 必须保留当前 exhausted 语义所需账号全集，而不是只用 schedulable active 集合。
}
```

- [ ] **Step 4: 写 projection 数据结构与局部计算**

```go
type OpenAIModelRoleView struct {
	CanonicalModel     string
	ExhaustedBaseIDs   []int64
	ReserveOverflowIDs []int64
}

type OpenAIModelSubsetProjection struct {
	Bucket            SchedulerBucket
	AccountReserveIDs map[int64]struct{}
	Models            map[string]OpenAIModelRoleView
}
```

- [ ] **Step 5: 实现账号级一致 reserve 提升与不变量**

```go
func BuildOpenAIModelSubsetProjection(inputs *OpenAIProjectionInputs) *OpenAIModelSubsetProjection {
	// 先按模型子集计算 exhausted base / reserve overflow candidate
	// 再做账号级 reserve lift
	// 同一模型子集里 exhausted base 与 reserve overflow 互斥
}

func liftModelSubsetReserveIdentities(local map[string]OpenAIModelRoleView, supportedModels map[int64][]string) map[string]OpenAIModelRoleView {
	// 仅提升已在某模型子集里属于 reserve overflow candidate 的账号
	// exhausted base 与 reserve overflow 在同一模型子集互斥
}
```

- [ ] **Step 6: 跑测试转 GREEN**

Run: `C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe test -tags unit ./internal/service -run "TestLoadOpenAIProjectionInputs_.*|TestBuildOpenAIModelSubsetProjection_.*" -count=1`

Expected: PASS。

- [ ] **Step 7: 提交这一小步**

```bash
git add backend/internal/service/openai_model_subset_projection.go backend/internal/service/openai_model_subset_projection_test.go backend/internal/service/scheduler_snapshot_service.go backend/internal/service/openai_gateway_service_test.go
git commit -m "fix(openai): 实现模型子集 reserve 派生投影"
```

## Task 3: 把 projection 接入 scheduler snapshot / cache 的单一 bundle 原子发布

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
	state, ok := svc.GetOpenAIBucketStateForTest(SchedulerBucket{GroupID: 2, Platform: PlatformOpenAI, Mode: SchedulerModeSingle})
	require.True(t, ok)
	require.Equal(t, int64(7), state.ProjectionVersion)
}

func TestSchedulerCache_OpenAIBucketStatePublishesAtomically(t *testing.T) {
	// 同一个 bucket 的 accounts/projection/version 必须共版切换，不能读到 mixed version
}
```

- [ ] **Step 2: 跑测试确认 RED**

Run: `C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe test -tags unit ./internal/service -run "TestSchedulerSnapshotHydration_PreservesOpenAIProjectionFields|TestSchedulerCache_OpenAIBucketStatePublishesAtomically|TestOpenAIModelSubsetProjectionIntegration_.*" -count=1`

Expected: FAIL，原因是 cache/snapshot 还不会按单一 bundle 读写 projection，也没有原子发布契约。

- [ ] **Step 3: 扩展 `SchedulerCache` 接口，改为单一 bucket state bundle**

```go
type OpenAISchedulerBucketState struct {
	Accounts          []*Account
	Projection        *OpenAIModelSubsetProjection
	ProjectionVersion int64
	BuiltAt           time.Time
}

type SchedulerCache interface {
	GetOpenAIBucketState(ctx context.Context, bucket SchedulerBucket) (*OpenAISchedulerBucketState, bool, error)
	SetOpenAIBucketState(ctx context.Context, bucket SchedulerBucket, state *OpenAISchedulerBucketState) error
}
```

说明：`projection_version` / `built_at` 的唯一分配者是 bucket-state publisher；`BuildOpenAIModelSubsetProjection(...)` 只产出 versionless 内容，发布时再统一回填到 bundle（以及如有需要的 projection 元数据只读镜像），读侧以 bundle 中的版本为真源。

- [ ] **Step 4: 在 `rebuildBucket(...)` 中生成并一次性发布 bundle**

```go
if bucket.Platform == PlatformOpenAI {
	inputs, _ := s.loadOpenAIProjectionInputs(ctx, bucket)
	projection := BuildOpenAIModelSubsetProjection(inputs)
	version := nextBucketProjectionVersion(bucket)
	builtAt := time.Now().UTC()
	state := &OpenAISchedulerBucketState{
		Accounts:          ptrAccounts(accounts),
		Projection:        projection,
		ProjectionVersion: version,
		BuiltAt:           builtAt,
	}
	if err := s.cache.SetOpenAIBucketState(ctx, bucket, state); err != nil { ... }
}
```

- [ ] **Step 5: 明确读契约，projection miss 必须 fail-closed**

```go
func (s *SchedulerSnapshotService) GetOpenAIBucketState(ctx context.Context, bucket SchedulerBucket) (*OpenAISchedulerBucketState, bool, error) {
	// 只能返回完整 bundle；若 snapshot/projection/version 不完整，返回 miss 或触发同步 rebuild
	// 绝不允许 gateway/scheduler 用 live account list 临时推 reserve
}
```

- [ ] **Step 6: 补 atomic publish/version 用例并转 GREEN**

Run: `C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe test -tags unit ./internal/service -run "TestSchedulerSnapshotHydration_PreservesOpenAIProjectionFields|TestSchedulerCache_OpenAIBucketStatePublishesAtomically|TestOpenAIModelSubsetProjectionIntegration_.*" -count=1`

Expected: PASS。

- [ ] **Step 7: 提交这一小步**

```bash
git add backend/internal/service/scheduler_cache.go backend/internal/repository/scheduler_cache.go backend/internal/service/scheduler_snapshot_service.go backend/internal/service/scheduler_snapshot_hydration_test.go backend/internal/service/openai_model_subset_projection_integration_test.go
git commit -m "fix(openai): 将模型子集投影接入快照发布"
```

## Task 4: 让请求热路径只消费 projection 结果

**Files:**
- Modify: `backend/internal/service/openai_gateway_service.go`
- Modify: `backend/internal/service/openai_account_scheduler.go`
- Modify: `backend/internal/service/openai_ws_forwarder.go`
- Modify: `backend/internal/service/openai_sticky_compat.go`
- Modify: `backend/internal/service/openai_ws_state_store.go`
- Modify: `backend/internal/service/openai_routing_observability.go`
- Modify: `backend/internal/service/openai_gateway_service_test.go`
- Modify: `backend/internal/service/openai_account_scheduler_test.go`
- Modify: `backend/internal/service/openai_ws_account_sticky_test.go`

- [ ] **Step 1: 先写请求态消费红灯测试**

```go
func TestOpenAIGatewayService_ListOpenAIExhaustedWithReserveOverlay_UsesProjectionInsteadOfLiveBuckets(t *testing.T) {
	// 构造 projection 中 gpt-5.6 只有 2 个账号，其中 1 个是 reserve
	// 即使 live account list 里全局 exhausted/reserve 为空，也应返回 projection 给出的 exhausted/reserve 结果
}

func TestOpenAIGatewayService_ProjectionMissFailsClosedWithoutLiveReserveFallback(t *testing.T) {
	// projection bundle 不完整时，必须 fail-closed 或同步重建，不能回退到 live derive reserve
}

func TestSelectByLoadBalance_TargetGroupAnyStillRejectsOverlayReserveFromProjection(t *testing.T) {
	// active/any 路径继续拒绝 overlay reserve；non-overlay reserve candidate 仍可按 active 身份参与
}

func TestPreviousResponseReserveBindingReadsSameProjectionKeyAsSysModel(t *testing.T) {
	// gpt-5.4-Sys 与 gpt-5.4 命中同一 projection key，但 routing target group 仍是 exhausted
}

func TestStickyReserveBinding_ProjectionVersionMismatchRebinds(t *testing.T) {
	// sticky / previous_response 命中旧 projection_version 时，必须失效或重绑，不能继续沿用旧 reserve 推导
}

func TestPreviousResponseNonOverlayReserveCandidateStillAcceptedForAny(t *testing.T) {
	// non-overlay reserve candidate 在 active/any 下仍可接受，但 selected_group 必须保持 active
}

func TestPreviousResponseNonOverlayReserveCandidateStillAcceptedForActive(t *testing.T) {
	// non-overlay reserve candidate 在 active 下也应保持可接受，且 selected_group 仍为 active
}
```

- [ ] **Step 2: 跑测试确认 RED**

Run: `C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe test -tags unit ./internal/service -run "TestOpenAIGatewayService_ListOpenAIExhaustedWithReserveOverlay_UsesProjectionInsteadOfLiveBuckets|TestOpenAIGatewayService_ProjectionMissFailsClosedWithoutLiveReserveFallback|TestSelectByLoadBalance_TargetGroupAnyStillRejectsOverlayReserveFromProjection|TestPreviousResponseReserveBindingReadsSameProjectionKeyAsSysModel|TestStickyReserveBinding_ProjectionVersionMismatchRebinds|TestPreviousResponseNonOverlayReserveCandidateStillAcceptedForAny|TestPreviousResponseNonOverlayReserveCandidateStillAcceptedForActive" -count=1`

Expected: FAIL，原因是当前热路径还在 live 算 reserve。

- [ ] **Step 3: 在 gateway/scheduler 中改为按 projection 取视图**

```go
func (s *OpenAIGatewayService) getOpenAIProjectionView(ctx context.Context, groupID *int64, requestedModel string) (*OpenAIModelRoleView, *OpenAISchedulerBucketState, error) {
	bucket := s.schedulerSnapshot.bucketFor(groupID, PlatformOpenAI, SchedulerModeSingle)
	state, hit, err := s.schedulerSnapshot.GetOpenAIBucketState(ctx, bucket)
	if err != nil || !hit || state == nil || state.Projection == nil {
		return nil, nil, ErrSchedulerCacheNotReady
	}
	// 只根据 projection 视图取 exhausted/reserve，不再 live 推导 reserve
}
```

- [ ] **Step 4: 保留现有 guardrail 与 sticky/previous_response 语义**

```go
// active/any 仍排斥 overlay reserve
// reserve affinity 只对 exhausted 请求有效
// old binding projection_version 不一致时失效或重绑
// `gpt-X-Sys` 与 base model 读取同一 canonical projection key，但外部 exhausted-class 语义不变
// failover recheck / previous_response / continuation 也必须只读 projection bundle
```

- [ ] **Step 5: 跑相关测试转 GREEN**

Run: `C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe test -tags unit ./internal/service -run "TestOpenAIGatewayService_ListOpenAIExhaustedWithReserveOverlay_UsesProjectionInsteadOfLiveBuckets|TestOpenAIGatewayService_ProjectionMissFailsClosedWithoutLiveReserveFallback|TestSelectByLoadBalance_Exhausted.*Reserve|TestSelectByLoadBalance_TargetGroupAnyNeverSelectsReserve|TestSelectByLoadBalance_TargetGroupActiveNeverSelectsReserve|TestPreviousResponseReserveBindingReadsSameProjectionKeyAsSysModel|TestPreviousResponseReserveBindingStillMatchesExhaustedClass|TestStickyReserveBinding_ProjectionVersionMismatchRebinds|TestPreviousResponseNonOverlayReserveCandidateStillAcceptedForAny|TestPreviousResponseNonOverlayReserveCandidateStillAcceptedForActive" -count=1`

Expected: PASS。

- [ ] **Step 6: 提交这一小步**

```bash
git add backend/internal/service/openai_gateway_service.go backend/internal/service/openai_account_scheduler.go backend/internal/service/openai_ws_forwarder.go backend/internal/service/openai_sticky_compat.go backend/internal/service/openai_ws_state_store.go backend/internal/service/openai_routing_observability.go backend/internal/service/openai_gateway_service_test.go backend/internal/service/openai_account_scheduler_test.go backend/internal/service/openai_ws_account_sticky_test.go
git commit -m "fix(openai): 让请求主链消费模型子集投影"
```

## Task 5: 把真实写路径纳入 projection refresh，并按严格 TDD 补完 unit/integration/线上验证矩阵

**Files:**
- Modify: `backend/internal/repository/account_repo.go`
- Modify: `backend/internal/repository/account_repo_integration_test.go`
- Modify: `backend/internal/service/openai_model_subset_projection_integration_test.go`
- Modify: `backend/internal/service/openai_model_subset_projection_test.go`
- Modify: `backend/internal/service/openai_gateway_service_test.go`
- Modify: `backend/internal/service/openai_ws_account_sticky_test.go`

- [ ] **Step 1: 先写真实触发链红灯测试**

```go
func TestUpdateCredentials_TriggersProjectionRefresh(t *testing.T) {}
func TestUpdateExtra_CodexSnapshotFieldsTriggerProjectionRefresh(t *testing.T) {}
func TestSetSchedulable_TriggersProjectionRefresh(t *testing.T) {}
func TestSetTempUnschedulable_TriggersProjectionRefresh(t *testing.T) {}
func TestClearTempUnschedulable_TriggersProjectionRefresh(t *testing.T) {}
func TestSetRateLimited_TriggersProjectionRefresh(t *testing.T) {}
func TestClearRateLimited_TriggersProjectionRefresh(t *testing.T) {}
func TestUnknownModel_IsConservativelyExcludedUntilCapabilitySnapshotRefresh(t *testing.T) {}
func TestProjectionVersion_ChangesAtomicallyWithBucketState(t *testing.T) {}
```

- [ ] **Step 2: 先跑真实写路径测试确认 RED**

Run: `C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe test -tags integration ./internal/repository -run "TestUpdateCredentials_TriggersProjectionRefresh|TestUpdateExtra_CodexSnapshotFieldsTriggerProjectionRefresh|TestSetSchedulable_TriggersProjectionRefresh|TestSetTempUnschedulable_TriggersProjectionRefresh|TestClearTempUnschedulable_TriggersProjectionRefresh|TestSetRateLimited_TriggersProjectionRefresh|TestClearRateLimited_TriggersProjectionRefresh" -count=1`

Expected: FAIL，原因是这些真实写路径目前仍是 scheduler-neutral、只做单账号 sync，或尚未进入 bundle projection refresh 链。

- [ ] **Step 3: 最小收口真实写路径实现，并让仓储 integration 先转 GREEN**

```go
func projectionRelevantExtraUpdate(updates map[string]any) bool {
	for key := range updates {
		switch key {
		case "codex_7d_used_percent", "codex_primary_used_percent", "openai_capability_explicit_models", "openai_capability_wildcard_rules", "openai_capability_default_allow":
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: 运行仓储 integration GREEN，并开始写第二轮高风险回归测试**

Run:

```bash
C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe test -tags integration ./internal/repository -run "TestUpdateCredentials_TriggersProjectionRefresh|TestUpdateExtra_CodexSnapshotFieldsTriggerProjectionRefresh|TestSetSchedulable_TriggersProjectionRefresh|TestSetTempUnschedulable_TriggersProjectionRefresh|TestClearTempUnschedulable_TriggersProjectionRefresh|TestSetRateLimited_TriggersProjectionRefresh|TestClearRateLimited_TriggersProjectionRefresh" -count=1
```

Expected: PASS。

然后补第二轮测试：

```go
func TestUnknownModel_EmptyMappingAndWildcardRemainConservativelyExcluded(t *testing.T) {}
func TestStickyReserveBinding_RebindsOnProjectionVersionChange(t *testing.T) {}
func TestPreviousResponseReserveBinding_InvalidatesWhenProjectionVersionChanges(t *testing.T) {}
```

- [ ] **Step 5: 先跑新增 unit + service integration 测试确认 RED**

Run:

```bash
C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe test -tags unit ./internal/service -run "TestUnknownModel_EmptyMappingAndWildcardRemainConservativelyExcluded|TestStickyReserveBinding_RebindsOnProjectionVersionChange|TestPreviousResponseReserveBinding_InvalidatesWhenProjectionVersionChanges" -count=1
C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe test -tags integration ./internal/service -run "TestUnknownModel_IsConservativelyExcludedUntilCapabilitySnapshotRefresh|TestProjectionVersion_ChangesAtomicallyWithBucketState" -count=1
```

Expected: FAIL，原因是 real write-path refresh 已收口，但 unknown-model refresh 闭环、bucket-state 原子版本切换、以及 sticky/previous_response 的 version-change 行为还未完全落地。

- [ ] **Step 6: 最小实现剩余的 refresh/version/rebind 逻辑**

```go
// 1. unknown-model 触发受控 refresh，并在 refresh 成功后 bump bucket-state version
// 2. sticky / previous_response binding 读到 projection_version mismatch 时失效或重绑
// 3. request 观测链带出新版本号，确保 integration 能断言版本切换
```

- [ ] **Step 7: 先跑新增 unit + service integration 测试转 GREEN**

Run:

```bash
C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe test -tags unit ./internal/service -run "TestUnknownModel_EmptyMappingAndWildcardRemainConservativelyExcluded|TestStickyReserveBinding_RebindsOnProjectionVersionChange|TestPreviousResponseReserveBinding_InvalidatesWhenProjectionVersionChanges" -count=1
C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe test -tags integration ./internal/service -run "TestUnknownModel_IsConservativelyExcludedUntilCapabilitySnapshotRefresh|TestProjectionVersion_ChangesAtomicallyWithBucketState" -count=1
```

Expected: PASS。

- [ ] **Step 8: 跑完整相关验证**

Run:

```bash
C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe test -tags integration ./internal/repository -count=1
C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe test -tags integration ./internal/service -run "TestUnknownModel_IsConservativelyExcludedUntilCapabilitySnapshotRefresh|TestProjectionVersion_ChangesAtomicallyWithBucketState" -count=1
C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe test -tags unit ./internal/service -run "TestBuildOpenAIModelSubsetProjection_.*|TestUnknownModel_EmptyMappingAndWildcardRemainConservativelyExcluded|TestOpenAIGatewayService_ListOpenAIExhaustedWithReserveOverlay_UsesProjectionInsteadOfLiveBuckets|TestOpenAIGatewayService_ProjectionMissFailsClosedWithoutLiveReserveFallback|TestSelectByLoadBalance_Exhausted.*Reserve|TestSelectByLoadBalance_TargetGroupAnyNeverSelectsReserve|TestSelectByLoadBalance_TargetGroupActiveNeverSelectsReserve|TestPreviousResponseReserveBindingReadsSameProjectionKeyAsSysModel|TestPreviousResponseReserveBindingStillMatchesExhaustedClass|TestStickyReserveBinding_ProjectionVersionMismatchRebinds|TestStickyReserveBinding_RebindsOnProjectionVersionChange|TestPreviousResponseReserveBinding_InvalidatesWhenProjectionVersionChanges|TestPreviousResponseNonOverlayReserveCandidateStillAcceptedForAny|TestPreviousResponseNonOverlayReserveCandidateStillAcceptedForActive" -count=1
C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe build ./cmd/server
git diff --check
```

Expected: 全部通过。

- [ ] **Step 9: 记录发版演练清单并提交最终实现**

```text
1. 先 pnpm install / pnpm build（如 worktree 缺 dist）
2. GOOS=linux GOARCH=amd64 CGO_ENABLED=0 构建 embed 二进制
3. 上传到 /tmp
4. 127.0.0.1:18081 smoke，至少验证 /health
5. 如条件允许，做一条真实新模型 exhausted-class 请求链路：
   - 初始无投影命中 -> 状态变化/能力刷新 -> projection_version 增长 -> exhausted-class 命中新的 reserve overflow 账号
6. 线上确认一次 unknown-model 保守排除/刷新闭环：
   - catalog 外模型首次请求被保守排除
   - 触发受控刷新
   - 请求日志里的 `projection_version` 递增
   - 后续请求只在 capability/catalog 明确证明后才命中新 projection
7. 提升正式版本并验证 /health
```

- [ ] **Step 10: 提交最终实现**

```bash
git add backend/internal/repository/account_repo.go backend/internal/repository/account_repo_integration_test.go backend/internal/service/openai_model_subset_projection_integration_test.go backend/internal/service/openai_model_subset_projection_test.go backend/internal/service/openai_gateway_service_test.go backend/internal/service/openai_ws_account_sticky_test.go
git commit -m "test(openai): 补齐模型子集 reserve 回归覆盖"
```
