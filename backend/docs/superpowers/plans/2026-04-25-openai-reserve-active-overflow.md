# OpenAI Reserve Active Overflow Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把 OpenAI reserve 的消费语义修正为“reserve 首先属于 active 身份的一部分，exhausted overflow 只是额外消费方式”，同时不放宽 projection miss / unknown-model 的 fail-closed 边界。

**Architecture:** 不重做 reserve 生成，只修改请求消费、sticky/previous_response/continuation 绑定与观测层。核心原则是：reserve 身份唯一来源仍然是当前 canonical model projection 的 `ReserveOverflowIDs`；`active/any` 与 `exhausted` 都可消费该身份，但 `target_group` 保持原始请求语义，`selected_group` 命中 reserve 一律写 `reserve`。

**Tech Stack:** Go, Gin, OpenAI gateway/service/scheduler, Redis sticky/response affinity state, existing projection bundle (`OpenAISchedulerBucketState`), existing OpenAI routing observability.

---

## 基线前提

当前新 worktree 的相关基线并非全绿：

- `C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe test -tags unit ./internal/service -run "TestSelectByLoadBalance_.*Reserve|TestOpenAIGatewayService_.*GPT55.*|TestOpenAIGatewayService_SelectAccountWithScheduler_PreviousResponse.*Reserve.*" -count=1`
- 当前已知旧红测：`TestSelectByLoadBalance_ReserveSelectedGroupWritesSharedAndAffinitySticky` 报 `scheduler cache not ready`

后续任务应先确认这条用例属于旧语义断言，还是实现接线缺口；不要把它误当成无关噪音跳过。

## File Map

- Modify: `backend/internal/service/openai_gateway_service.go`
  - 统一 active/any/exhausted 对 reserve 的请求期消费语义；保留 projection miss / unknown-model fail-closed 分类。
- Modify: `backend/internal/service/openai_account_scheduler.go`
  - 调整 load-balance、binding helper、sticky、previous_response 路径对 reserve 的接纳/拒绝逻辑。
- Modify: `backend/internal/service/openai_ws_forwarder.go`
  - 调整 response affinity / continuation 对 reserve binding 的兼容读取与命中逻辑。
- Modify: `backend/internal/service/openai_sticky_compat.go`
  - 统一 reserve binding 读写、compat helper 与 affinity domain 兼容规则。
- Modify: `backend/internal/service/openai_routing_observability.go`
  - 确保 active/any/exhausted 命中 reserve 时 `target_group` 与 `selected_group` 双层观测一致。
- Modify: `backend/internal/service/ops_openai_routing_stats.go`
  - 如需要，补齐 `target_group × selected_group` 组合的聚合/读取断言。
- Modify: `backend/internal/service/ops_request_details.go`
  - 如需要，补齐 request details 中 active/any/exhausted 命中 reserve 的断言。
- Modify: `backend/internal/service/openai_gateway_service_test.go`
  - 增加 active/any 对 reserve 的请求消费与 fail-closed 负向回归。
- Modify: `backend/internal/service/openai_account_scheduler_test.go`
  - 增加/修正 load-balance、sticky、previous_response 的 reserve 行为矩阵。
- Modify: `backend/internal/service/openai_ws_account_sticky_test.go`
  - 覆盖旧 `affinity_domain=exhausted` reserve binding 的兼容读取，以及 WS continuation reserve 语义。
- Modify: `backend/internal/handler/openai_gateway_handler_test.go`
  - 如有必要，锁住 request-level `routing_target_group` / `routing_selected_group` 观测输出。

## Task 1: 锁定 reserve active 消费语义的主路径回归

**Files:**
- Modify: `backend/internal/service/openai_gateway_service_test.go`
- Modify: `backend/internal/service/openai_account_scheduler_test.go`

- [ ] **Step 1: 先写 fail-first 测试，明确旧行为反转**

```go
func TestSelectByLoadBalance_TargetGroupAnyCanSelectReserve(t *testing.T) {}
func TestSelectByLoadBalance_TargetGroupActiveCanSelectReserve(t *testing.T) {}
func TestOpenAIGatewayService_PreviousResponseReserveAcceptedForAny(t *testing.T) {}
func TestOpenAIGatewayService_PreviousResponseReserveAcceptedForActive(t *testing.T) {}
func TestSelectByLoadBalance_ReserveSelectedGroupWritesReserveForActiveAny(t *testing.T) {}
```

- [ ] **Step 2: 跑测试确认 RED**

Run:

```bash
C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe test -tags unit ./internal/service -run "TestSelectByLoadBalance_TargetGroup(Any|Active)CanSelectReserve|TestOpenAIGatewayService_PreviousResponseReserveAcceptedFor(Any|Active)|TestSelectByLoadBalance_ReserveSelectedGroupWritesReserveForActiveAny" -count=1
```

Expected: FAIL。当前旧逻辑仍会拒绝 active/any 命中 reserve，或把 `selected_group` 写成 `active`。

- [ ] **Step 3: 写最小实现，只放宽 reserve 消费，不改生成逻辑**

```go
// 原则：reserve 身份唯一来源仍然是 projection view 的 ReserveOverflowIDs。
// 只改消费侧：
// 1. active/any 不再拒绝 reserve
// 2. 命中 reserve 后 selected_group 一律写 reserve
// 3. exhausted 现有 overflow 规则保持不变
```

- [ ] **Step 4: 跑测试转 GREEN**

Run the same command as Step 2.

Expected: PASS。

- [ ] **Step 5: 提交这一小步**

```bash
git add backend/internal/service/openai_gateway_service_test.go backend/internal/service/openai_account_scheduler_test.go backend/internal/service/openai_gateway_service.go backend/internal/service/openai_account_scheduler.go
git commit -m "fix(openai): 放开 active 与 any 对 reserve 的消费"
```

## Task 2: 修正 sticky / previous_response / continuation 的 reserve binding 兼容

**Files:**
- Modify: `backend/internal/service/openai_account_scheduler.go`
- Modify: `backend/internal/service/openai_sticky_compat.go`
- Modify: `backend/internal/service/openai_ws_forwarder.go`
- Modify: `backend/internal/service/openai_ws_account_sticky_test.go`
- Modify: `backend/internal/service/openai_account_scheduler_test.go`

- [ ] **Step 1: 先写 binding 兼容矩阵测试**

```go
func TestStickyReserveBindingAcceptedForActive(t *testing.T) {}
func TestStickyReserveBindingAcceptedForAny(t *testing.T) {}
func TestStickyReserveBindingAcceptedForExhaustedWritesReserveDomain(t *testing.T) {}
func TestPreviousResponseReserveBindingAcceptedForActive(t *testing.T) {}
func TestPreviousResponseReserveBindingAcceptedForAny(t *testing.T) {}
func TestPreviousResponseReserveBindingAcceptedForExhaustedWritesReserveDomain(t *testing.T) {}
func TestLegacyReserveBindingWithExhaustedAffinityDomainStillReadable(t *testing.T) {}
func TestWSContinuationReserveBindingAcceptedForActiveAny(t *testing.T) {}
func TestWSContinuationReserveBindingAcceptedForExhaustedWritesReserveDomain(t *testing.T) {}
```

- [ ] **Step 2: 跑测试确认 RED**

Run:

```bash
C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe test -tags unit ./internal/service -run "Test(Sticky|PreviousResponse|WSContinuation).*Reserve.*|TestLegacyReserveBindingWithExhaustedAffinityDomainStillReadable" -count=1
```

Expected: FAIL。当前旧 binding 仍可能把 `affinity_domain=exhausted` 当成 active/any 下的拒绝理由，且 exhausted 新写入也可能仍沿用旧 `affinity_domain=exhausted`。

- [ ] **Step 3: 最小实现兼容读取与新写入规则**

```go
// 新写入：active/any/exhausted 命中 reserve 时统一写 affinity_domain=reserve
// 读取兼容：旧 selected_group=reserve + affinity_domain=exhausted 的 binding 只要 projection 元数据一致，仍可读
// 继续要求 projection_version / model_key / built_at 一致
// 必须显式改这些中心 helper：normalizeOpenAIAffinityDomain、newOpenAIAffinityBinding、isOpenAIReserveAffinityBinding、resolveOpenAISelectedGroupFromBindingOrAccount
```

- [ ] **Step 4: 跑测试转 GREEN**

Run the same command as Step 2.

Expected: PASS。

- [ ] **Step 5: 提交这一小步**

```bash
git add backend/internal/service/openai_account_scheduler.go backend/internal/service/openai_sticky_compat.go backend/internal/service/openai_ws_forwarder.go backend/internal/service/openai_ws_account_sticky_test.go backend/internal/service/openai_account_scheduler_test.go
git commit -m "fix(openai): 调整 reserve 绑定的 active 兼容语义"
```

## Task 3: 锁住 projection miss / unknown-model fail-closed 不被放宽

**Files:**
- Modify: `backend/internal/service/openai_gateway_service.go`
- Modify: `backend/internal/service/openai_account_scheduler.go`
- Modify: `backend/internal/service/openai_gateway_service_test.go`
- Modify: `backend/internal/service/openai_account_scheduler_test.go`
- Modify: `backend/internal/service/openai_ws_account_sticky_test.go`
- Modify: `backend/internal/service/openai_ws_forwarder.go`

- [ ] **Step 1: 先写负向测试**

```go
func TestSelectByLoadBalance_AnyReserveStillFailsClosedOnProjectionMiss(t *testing.T) {}
func TestSelectByLoadBalance_ActiveReserveStillFailsClosedOnProjectionMiss(t *testing.T) {}
func TestSelectByLoadBalance_AnyReserveStillFailsClosedOnCacheNotReady(t *testing.T) {}
func TestSelectByLoadBalance_ActiveReserveStillFailsClosedOnCacheNotReady(t *testing.T) {}
func TestSelectByLoadBalance_AnyReserveStillFailsClosedOnUnknownModelMiss(t *testing.T) {}
func TestSelectByLoadBalance_ActiveReserveStillFailsClosedOnUnknownModelMiss(t *testing.T) {}
func TestPreviousResponseReserveStillFailsClosedOnProjectionMiss(t *testing.T) {}
func TestPreviousResponseReserveStillFailsClosedOnUnknownModelMiss(t *testing.T) {}
func TestPreviousResponseReserveStillFailsClosedOnCacheNotReady(t *testing.T) {}
func TestStickyReserveStillFailsClosedOnProjectionMiss(t *testing.T) {}
func TestStickyReserveStillFailsClosedOnUnknownModelMiss(t *testing.T) {}
func TestStickyReserveStillFailsClosedOnCacheNotReady(t *testing.T) {}
func TestWSContinuationReserveStillFailsClosedOnProjectionMiss(t *testing.T) {}
func TestWSContinuationReserveStillFailsClosedOnUnknownModelMiss(t *testing.T) {}
func TestWSContinuationReserveStillFailsClosedOnCacheNotReady(t *testing.T) {}
```

- [ ] **Step 2: 跑测试确认 RED**

Run:

```bash
C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe test -tags unit ./internal/service -run "Test(SelectByLoadBalance_.*Reserve.*(ProjectionMiss|UnknownModelMiss|CacheNotReady)|PreviousResponseReserveStillFailsClosedOn(ProjectionMiss|UnknownModelMiss|CacheNotReady)|StickyReserveStillFailsClosedOn(ProjectionMiss|UnknownModelMiss|CacheNotReady)|WSContinuationReserveStillFailsClosedOn(ProjectionMiss|UnknownModelMiss|CacheNotReady))" -count=1
```

Expected: FAIL，或至少能证明当前放宽 reserve 后边界没有被显式锁住。

- [ ] **Step 3: 最小实现，只修分类不放宽 fail-closed**

```go
// reserve 可被 active/any 消费 != projection miss 可 fallback 到 live reserve
// source-known miss、unknown-model miss、cache-not-ready 继续走各自既有受控边界
```

- [ ] **Step 4: 跑测试转 GREEN**

Run the same command as Step 2.

Expected: PASS。

- [ ] **Step 5: 提交这一小步**

```bash
git add backend/internal/service/openai_gateway_service_test.go backend/internal/service/openai_account_scheduler_test.go backend/internal/service/openai_ws_account_sticky_test.go backend/internal/service/openai_ws_forwarder.go backend/internal/service/openai_gateway_service.go backend/internal/service/openai_account_scheduler.go
git commit -m "fix(openai): 保持 reserve 放宽下的 fail-closed 边界"
```

## Task 4: 补齐 request/usage/ops 观测层断言

**Files:**
- Modify: `backend/internal/service/openai_routing_observability.go`
- Modify: `backend/internal/service/ops_openai_routing_stats.go`
- Modify: `backend/internal/service/ops_request_details.go`
- Modify: `backend/internal/service/openai_routing_observability_test.go`
- Modify: `backend/internal/service/ops_openai_routing_stats_test.go`
- Modify: `backend/internal/repository/ops_repo_openai_routing_stats.go`
- Modify: `backend/internal/repository/ops_repo_request_details.go`
- Modify: `backend/internal/repository/ops_repo_openai_routing_stats_test.go`
- Modify: `backend/internal/repository/ops_repo_request_details_test.go`
- Modify: `backend/internal/handler/openai_gateway_handler_test.go`

- [ ] **Step 1: 先写观测回归测试**

```go
func TestRoutingSnapshot_ActiveReserveKeepsTargetButWritesSelectedReserve(t *testing.T) {}
func TestRoutingSnapshot_AnyReserveKeepsTargetButWritesSelectedReserve(t *testing.T) {}
func TestRoutingSnapshot_ExhaustedReserveKeepsTargetButWritesSelectedReserve(t *testing.T) {}
func TestUsageAndOps_ActiveReservePersistsTargetAndSelectedGroup(t *testing.T) {}
func TestUsageAndOps_AnyReservePersistsAnyTargetAndReserveSelectedGroup(t *testing.T) {}
func TestUsageAndOps_ExhaustedReservePersistsTargetAndSelectedGroup(t *testing.T) {}
```

- [ ] **Step 2: 跑测试确认 RED**

Run:

```bash
C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe test -tags unit ./internal/handler -run "TestRoutingSnapshot_.*Reserve.*" -count=1
C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe test -tags unit ./internal/service -run "TestUsageAndOps_(Active|Any|Exhausted)Reserve.*|TestOpenAIRoutingSnapshot_.*" -count=1
C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe test ./internal/repository -run "TestOpsRepo(OpenAIRoutingStats|RequestDetails)_.*Reserve.*" -count=1
```

Expected: FAIL，如果当前快照/观测仍把 active/any 命中的 reserve 写成 `selected_group=active`、把 any 丢成空语义、或改写了 target。

- [ ] **Step 3: 最小实现观测修正**

```go
// 命中 reserve 的所有请求：selected_group=reserve
// routing_target_group 保持请求原始语义
// usage / request details / ops 聚合层必须能区分 active/any/exhausted -> reserve
```

- [ ] **Step 4: 跑测试转 GREEN**

Run the same commands as Step 2.

Expected: PASS。

- [ ] **Step 5: 提交这一小步**

```bash
git add backend/internal/service/openai_routing_observability.go backend/internal/service/ops_openai_routing_stats.go backend/internal/service/ops_request_details.go backend/internal/service/openai_routing_observability_test.go backend/internal/service/ops_openai_routing_stats_test.go backend/internal/repository/ops_repo_openai_routing_stats.go backend/internal/repository/ops_repo_request_details.go backend/internal/repository/ops_repo_openai_routing_stats_test.go backend/internal/repository/ops_repo_request_details_test.go backend/internal/handler/openai_gateway_handler_test.go
git commit -m "fix(openai): 统一 reserve 命中的观测语义"
```

## Task 5: 做整体验证并准备发版检查清单

**Files:**
- Modify: `backend/internal/service/openai_gateway_service_test.go`
- Modify: `backend/internal/service/openai_account_scheduler_test.go`
- Modify: `backend/internal/service/openai_ws_account_sticky_test.go`
- Modify: `backend/internal/handler/openai_gateway_handler_test.go`

- [ ] **Step 1: 补全总回归矩阵用例**

```go
func TestReserveSemanticShift_OldRejectAnyNowAccepts(t *testing.T) {}
func TestReserveSemanticShift_OldRejectActiveNowAccepts(t *testing.T) {}
func TestReserveSemanticShift_OldPreviousResponseReserveRejectedForAnyNowAccepts(t *testing.T) {}
func TestReserveSemanticShift_OldPreviousResponseReserveRejectedForActiveNowAccepts(t *testing.T) {}
func TestReserveSemanticShift_GPT54SysAndGPT55SysDoNotRegress(t *testing.T) {}
func TestReserveSemanticShift_GPT55ActiveHitsReserveAndReturnsSelectedReserve(t *testing.T) {}
func TestReserveSemanticShift_OldReserveSharedStickyBindingRejectedForAnyNowAccepts(t *testing.T) {}
func TestReserveSemanticShift_OldReserveSharedStickyBindingRejectedForActiveNowAccepts(t *testing.T) {}
```

- [ ] **Step 2: 跑总回归测试确认 RED**

Run:

```bash
C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe test -tags unit ./internal/service -run "TestReserveSemanticShift_.*|TestOpenAIGatewayService_(PreviousResponseReserveAcceptedForAny|PreviousResponseReserveAcceptedForActive|SelectAccountWithScheduler_PreviousResponseReserveRejectedForAny|SelectAccountWithScheduler_PreviousResponseReserveRejectedForActive|SelectAccountWithScheduler_ReserveSharedStickyBindingRejectedForAny|SelectAccountWithScheduler_ReserveSharedStickyBindingRejectedForActive|.*GPT55.*)|TestSelectByLoadBalance_(TargetGroupAnyCanSelectReserve|TargetGroupActiveCanSelectReserve|TargetGroupAnyNeverSelectsReserve|TargetGroupActiveNeverSelectsReserve|ReserveSelectedGroupWritesReserveForActiveAny|.*GPT55.*|.*Reserve.*(ProjectionMiss|UnknownModelMiss|CacheNotReady))|Test(Sticky|PreviousResponse|WSContinuation).*Reserve.*|TestLegacyReserveBindingWithExhaustedAffinityDomainStillReadable" -count=1
```

Expected: 若前面任务未完全覆盖，这里应先出现失败；如果已因前序任务全部转绿，也要把这组命令作为总验收门槛保留。

- [ ] **Step 3: 补最小缺口并转 GREEN**

```go
// 只修 Step 2 暴露的剩余小缺口；不新增新语义。
```

- [ ] **Step 4: 运行完整验证**

Run:

```bash
C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe test ./internal/handler -count=1
C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe test -tags unit ./internal/service -run "TestReserveSemanticShift_.*|TestOpenAIGatewayService_(PreviousResponseReserveAcceptedForAny|PreviousResponseReserveAcceptedForActive|SelectAccountWithScheduler_PreviousResponseReserveRejectedForAny|SelectAccountWithScheduler_PreviousResponseReserveRejectedForActive|SelectAccountWithScheduler_ReserveSharedStickyBindingRejectedForAny|SelectAccountWithScheduler_ReserveSharedStickyBindingRejectedForActive|.*GPT55.*)|TestSelectByLoadBalance_(TargetGroupAnyCanSelectReserve|TargetGroupActiveCanSelectReserve|TargetGroupAnyNeverSelectsReserve|TargetGroupActiveNeverSelectsReserve|ReserveSelectedGroupWritesReserveForActiveAny|.*GPT55.*|AnyReserveStillFailsClosedOnProjectionMiss|ActiveReserveStillFailsClosedOnProjectionMiss|AnyReserveStillFailsClosedOnUnknownModelMiss|ActiveReserveStillFailsClosedOnUnknownModelMiss|AnyReserveStillFailsClosedOnCacheNotReady|ActiveReserveStillFailsClosedOnCacheNotReady)|Test(Sticky|PreviousResponse|WSContinuation).*Reserve.*|TestLegacyReserveBindingWithExhaustedAffinityDomainStillReadable|TestUsageAndOps_(Active|Any|Exhausted)Reserve.*" -count=1
C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe test ./internal/repository -run "TestOpsRepo(OpenAIRoutingStats|RequestDetails)_.*Reserve.*" -count=1
C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe build ./cmd/server
git diff --check
```

Expected: 全部通过。

- [ ] **Step 5: 发版检查清单**

```text
1. 若 worktree 缺 dist，先 `pnpm install` / `pnpm build`
2. GOOS=linux GOARCH=amd64 CGO_ENABLED=0 构建 embed 二进制
3. 上传到 /tmp
4. 127.0.0.1:18081 smoke，至少验证 /health
5. 共享生产 Redis smoke 同时验证：
   - plain `gpt-5.5` -> active/any 可命中 reserve，不再 503
   - `GPT-5.5-Sys` -> 继续 200
   - `GPT-5.4-Sys` -> 继续 200
6. 观察 request/usage/ops：
    - target_group 保持原始语义
    - selected_group 命中 reserve 一律写 reserve
    - `any` 在外部观测里必须可区分，不得退化成空值/丢失语义
    - 命中的 reserve 账号确实来自当前 canonical projection 的 `ReserveOverflowIDs`
7. 对三条链路分别记录 request id，并逐条核验：
   - plain `gpt-5.5` active/any
   - `GPT-5.5-Sys`
   - `GPT-5.4-Sys`
   每条都核验 `target_group`、`selected_group`、selected account、projection metadata
8. 额外做旧 binding 兼容 smoke：
   - 旧 `selected_group=reserve + affinity_domain=exhausted` binding 在 active/any/exhausted 下继续可读
   - 新写入统一变为 `affinity_domain=reserve`
   - `previous_response` 与 WS continuation 都验证一遍
9. 正式切换后再次复测以上三条链路
```

- [ ] **Step 6: 提交最终实现**

```bash
git add backend/internal/service/openai_gateway_service_test.go backend/internal/service/openai_account_scheduler_test.go backend/internal/service/openai_ws_account_sticky_test.go backend/internal/handler/openai_gateway_handler_test.go
git commit -m "test(openai): 补齐 reserve active 语义回归覆盖"
```
