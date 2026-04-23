# OpenAI Retry Account Switch Penalty Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让 OpenAI 当前 HTTP failover 主链在同账号连续两次可重试失败后积极换号，并在同账号连续五次可重试失败后通过现有 `temp_unschedulable` 体系将其暂时踢出路由十分钟。

**Architecture:** 保持首轮调度与 target-group 语义不变，把“请求内换号”落在 OpenAI handler 的 retry loop，把“跨请求惩罚”落在 OpenAI 专属运行时 tracker，并只在达到阈值时复用现有 `SetTempUnschedulable` 与 scheduler snapshot/outbox 体系。恢复语义通过一个轻量的到期扫描触发快照刷新，而不是引入新的数据库表或分布式实时计数系统。

**Tech Stack:** Go, Gin, existing OpenAI gateway handlers/services, scheduler snapshot service, PostgreSQL-backed account repo, existing temp-unschedulable + outbox pipeline.

---

## File Map

- Modify: `backend/internal/handler/openai_gateway_handler.go`
  - Responses 与 OpenAI Messages failover loop，加入请求级账号 streak / `excludedIDs` 策略。
- Modify: `backend/internal/handler/openai_chat_completions.go`
  - Chat Completions failover loop，保持和 Responses 主链同一行为。
- Modify: `backend/internal/handler/openai_gateway_handler_test.go`
  - 增加请求级连续失败换号、预算受限、sticky/previous-response 不回归测试。
- Create: `backend/internal/service/openai_retry_penalty.go`
  - 封装账号级连续失败 streak、`penalty_pending`、成功清零、阈值 crossing 与 structured reason 构造。
- Create: `backend/internal/service/openai_retry_penalty_test.go`
  - 纯 service 层测试 streak 状态机、写库失败重试、成功清零、账号隔离。
- Modify: `backend/internal/service/openai_gateway_service.go`
  - 暴露 OpenAI 专属 retry penalty tracker 的调用入口，供 handler 在成功/失败时上报。
- Modify: `backend/internal/service/scheduler_snapshot_service.go`
  - 增加 temp-unsched 到期恢复扫描与 `account_id + until` 去重。
- Create: `backend/internal/service/scheduler_snapshot_penalty_recovery_test.go`
  - 验证到期恢复触发刷新、幂等去重、恢复上界的关键链路。
- Modify: `backend/internal/service/ratelimit_service.go`
  - 如需要，抽出可复用的 `temp_unschedulable` reason 构造 helper，避免 OpenAI penalty 与现有 temp-unsched 格式分叉。
- Modify: `backend/internal/repository/account_repo.go`
  - 仅在需要新增最小仓储辅助查询时改动；若现有接口足够则不动。

## Task 1: 锁定请求级“连续两次失败后换号”语义

**Files:**
- Modify: `backend/internal/handler/openai_gateway_handler.go`
- Modify: `backend/internal/handler/openai_chat_completions.go`
- Modify: `backend/internal/handler/openai_gateway_handler_test.go`

- [ ] **Step 1: 在 handler 测试里先写失败用例**

```go
func TestOpenAIResponsesFailover_ExcludesAccountAfterTwoRetryableFailures(t *testing.T) {
	var seenExcluded []map[int64]struct{}
	scheduler := &openAIAccountSchedulerStub{
		selectFn: func(ctx context.Context, req service.OpenAIAccountScheduleRequest) (*service.AccountSelectionResult, service.OpenAIAccountScheduleDecision, error) {
			copied := map[int64]struct{}{}
			for id := range req.ExcludedIDs {
				copied[id] = struct{}{}
			}
			seenExcluded = append(seenExcluded, copied)
			if len(seenExcluded) < 3 {
				return &service.AccountSelectionResult{Account: &service.Account{ID: 1001, Name: "A", Platform: service.PlatformOpenAI}}, service.OpenAIAccountScheduleDecision{}, nil
			}
			return &service.AccountSelectionResult{Account: &service.Account{ID: 1002, Name: "B", Platform: service.PlatformOpenAI}}, service.OpenAIAccountScheduleDecision{}, nil
		},
	}

	// 配置 gatewayService.Forward：A 前两次返回 RetryableOnSameAccount=true 的 UpstreamFailoverError，第三次成功。
	// 断言第三轮选号时 excludedIDs 含 1001，最终命中账号 1002。
}
```

- [ ] **Step 2: 只跑新用例，确认 RED**

Run: `C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe test ./internal/handler -run TestOpenAIResponsesFailover_ExcludesAccountAfterTwoRetryableFailures -count=1`

Expected: FAIL，原因是当前实现仍会沿用 `pool_mode_retry_count`，第三次前不会把账号 A 放进 `excludedIDs`。

- [ ] **Step 3: 在 Responses / Messages / Chat Completions failover loop 引入请求级 streak 结构**

```go
type openAIRequestRetryState struct {
	accountRetryableFailures map[int64]int
}

func (s *openAIRequestRetryState) RecordRetryableFailure(accountID int64) int {
	if s.accountRetryableFailures == nil {
		s.accountRetryableFailures = make(map[int64]int)
	}
	s.accountRetryableFailures[accountID]++
	return s.accountRetryableFailures[accountID]
}

func (s *openAIRequestRetryState) ShouldExclude(accountID int64) bool {
	return s.accountRetryableFailures[accountID] >= 2
}
```

- [ ] **Step 4: 用最小实现接入现有 retry loop**

```go
retryState := &openAIRequestRetryState{accountRetryableFailures: map[int64]int{}}

// 在 failoverErr.RetryableOnSameAccount 分支里：
streak := retryState.RecordRetryableFailure(account.ID)
retryLimit := account.GetPoolModeRetryCount()
if streak < 2 && sameAccountRetryCount[account.ID] < retryLimit {
	sameAccountRetryCount[account.ID]++
	// 保留现有延迟与 continue
	continue
}

failedAccountIDs[account.ID] = struct{}{}
```

- [ ] **Step 5: 补一个“小预算优先”测试**

```go
func TestOpenAIResponsesFailover_RespectsSmallerRetryBudget(t *testing.T) {
	// 账号 A 的 pool_mode_retry_count = 1
	// 第一次失败后允许同账号重试一次；预算耗尽后直接换号，不为了凑 streak=2 扩大总重试次数。
}
```

- [ ] **Step 6: 跑 handler 用例确认 GREEN**

Run: `C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe test ./internal/handler -run "TestOpenAIResponsesFailover_ExcludesAccountAfterTwoRetryableFailures|TestOpenAIResponsesFailover_RespectsSmallerRetryBudget" -count=1`

Expected: PASS。

- [ ] **Step 7: 提交这一小步**

```bash
git add backend/internal/handler/openai_gateway_handler.go backend/internal/handler/openai_chat_completions.go backend/internal/handler/openai_gateway_handler_test.go
git commit -m "fix(openai): 在连续重试失败后积极换号"
```

## Task 2: 实现账号级连续失败惩罚状态机

**Files:**
- Create: `backend/internal/service/openai_retry_penalty.go`
- Create: `backend/internal/service/openai_retry_penalty_test.go`
- Modify: `backend/internal/service/openai_gateway_service.go`

- [ ] **Step 1: 先写状态机测试**

```go
func TestOpenAIRetryPenaltyTracker_PenalizesAfterFiveRetryableFailures(t *testing.T) {
	tracker := NewOpenAIRetryPenaltyTracker(time.Minute * 15)
	for i := 0; i < 4; i++ {
		decision := tracker.RecordFailure(42, openAIRetryPenaltyFailureMeta{TriggerKind: "upstream_429"}, time.Now())
		require.False(t, decision.ShouldPenalize)
	}
	decision := tracker.RecordFailure(42, openAIRetryPenaltyFailureMeta{TriggerKind: "upstream_429"}, time.Now())
	require.True(t, decision.ShouldPenalize)
	require.Equal(t, 5, decision.FailureCount)
}

func TestOpenAIRetryPenaltyTracker_SuccessClearsOnlyThatAccount(t *testing.T) {
	tracker := NewOpenAIRetryPenaltyTracker(time.Minute * 15)
	tracker.RecordFailure(42, openAIRetryPenaltyFailureMeta{}, time.Now())
	tracker.RecordFailure(43, openAIRetryPenaltyFailureMeta{}, time.Now())
	tracker.RecordSuccess(42)
	require.Zero(t, tracker.CurrentStreak(42))
	require.Equal(t, 1, tracker.CurrentStreak(43))
}
```

- [ ] **Step 2: 运行测试确认 RED**

Run: `C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe test -tags unit ./internal/service -run "TestOpenAIRetryPenaltyTracker_.*" -count=1`

Expected: FAIL，原因是 tracker 尚不存在。

- [ ] **Step 3: 写最小 tracker 实现**

```go
type openAIRetryPenaltyState struct {
	ConsecutiveFailures int
	ExpiresAt           time.Time
	PenaltyPending      bool
	LastPenaltyUntil    time.Time
}

type OpenAIRetryPenaltyTracker struct {
	mu    sync.Mutex
	streakTTL time.Duration
	state map[int64]*openAIRetryPenaltyState
}
```

- [ ] **Step 4: 定义失败/成功/写库结果状态流转**

```go
func (t *OpenAIRetryPenaltyTracker) RecordFailure(accountID int64, meta openAIRetryPenaltyFailureMeta, now time.Time) openAIRetryPenaltyDecision
func (t *OpenAIRetryPenaltyTracker) RecordSuccess(accountID int64)
func (t *OpenAIRetryPenaltyTracker) MarkPenaltyWriteResult(accountID int64, until time.Time, ok bool)
```

要求：
- 成功只清当前进程本地 streak。
- streak 15 分钟无新失败自动过期。
- 达阈值后进入 `penalty_pending`，写库成功才真正清空；写库失败不丢状态。

- [ ] **Step 5: 让 OpenAIGatewayService 暴露统一入口**

```go
func (s *OpenAIGatewayService) RecordOpenAIRetryableFailure(accountID int64, triggerKind string) openAIRetryPenaltyDecision
func (s *OpenAIGatewayService) RecordOpenAIRetrySuccess(accountID int64)
func (s *OpenAIGatewayService) MarkOpenAIRetryPenaltyWriteResult(accountID int64, until time.Time, ok bool)
```

- [ ] **Step 6: 跑状态机测试确认 GREEN**

Run: `C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe test -tags unit ./internal/service -run "TestOpenAIRetryPenaltyTracker_.*" -count=1`

Expected: PASS。

- [ ] **Step 7: 提交这一小步**

```bash
git add backend/internal/service/openai_retry_penalty.go backend/internal/service/openai_retry_penalty_test.go backend/internal/service/openai_gateway_service.go
git commit -m "fix(openai): 增加账号失败惩罚状态机"
```

## Task 3: 把惩罚接入 temp_unschedulable 与结构化 reason

**Files:**
- Modify: `backend/internal/handler/openai_gateway_handler.go`
- Modify: `backend/internal/handler/openai_chat_completions.go`
- Modify: `backend/internal/service/ratelimit_service.go`
- Modify: `backend/internal/service/openai_retry_penalty_test.go`

- [ ] **Step 1: 先写“第 5 次失败落库 + 写库失败不丢状态”的测试**

```go
func TestOpenAIRetryPenaltyTracker_WriteFailureKeepsPendingState(t *testing.T) {
	tracker := NewOpenAIRetryPenaltyTracker(time.Minute * 15)
	decision := openAIRetryPenaltyDecision{ShouldPenalize: true, FailureCount: 5}
	tracker.MarkPenaltyWriteResult(42, time.Now().Add(10*time.Minute), false)
	require.True(t, tracker.IsPenaltyPending(42))
}
```

- [ ] **Step 2: 在 handler failover 成功/失败分支接入 tracker**

```go
decision := h.gatewayService.RecordOpenAIRetryableFailure(account.ID, fmt.Sprintf("upstream_%d", failoverErr.StatusCode))
if decision.ShouldPenalize {
	until := time.Now().Add(10 * time.Minute)
	reason := buildOpenAIRetryPenaltyReason(decision, until)
	ok := h.gatewayService.SetTempUnschedulableForOpenAIPenalty(ctx, account.ID, until, reason)
	h.gatewayService.MarkOpenAIRetryPenaltyWriteResult(account.ID, until, ok)
}

// 成功路径
h.gatewayService.RecordOpenAIRetrySuccess(account.ID)
```

- [ ] **Step 3: reason 与日志字段统一机器可读**

```go
type OpenAIRetryPenaltyReason struct {
	PenaltyKind  string `json:"penalty_kind"`
	FailureCount int    `json:"failure_count"`
	TriggerKind  string `json:"trigger_kind"`
	InstanceID   string `json:"instance_id,omitempty"`
	WriteResult  string `json:"write_result"`
}
```

- [ ] **Step 4: 运行定向测试**

Run: `C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe test -tags unit ./internal/service -run "TestOpenAIRetryPenaltyTracker_.*" -count=1`

Expected: PASS。

- [ ] **Step 5: 提交这一小步**

```bash
git add backend/internal/handler/openai_gateway_handler.go backend/internal/handler/openai_chat_completions.go backend/internal/service/ratelimit_service.go backend/internal/service/openai_retry_penalty_test.go
git commit -m "fix(openai): 将连续失败惩罚接入临时退出路由"
```

## Task 4: 增加惩罚到期恢复扫描与快照刷新

**Files:**
- Modify: `backend/internal/service/scheduler_snapshot_service.go`
- Create: `backend/internal/service/scheduler_snapshot_penalty_recovery_test.go`

- [ ] **Step 1: 先写恢复扫描测试**

```go
func TestPenaltyRecoveryScan_RefreshesExpiredTempUnschedAccountOnce(t *testing.T) {
	// 构造一个 temp_unschedulable_until 已过期账号
	// 断言扫描只触发一次 account_changed / rebuild，即使轮询多次也不重复。
}
```

- [ ] **Step 2: 运行测试确认 RED**

Run: `C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe test -tags unit ./internal/service -run TestPenaltyRecoveryScan_RefreshesExpiredTempUnschedAccountOnce -count=1`

Expected: FAIL，原因是当前没有 penalty recovery scan。

- [ ] **Step 3: 在 SchedulerSnapshotService 增加轻量恢复 worker**

```go
func (s *SchedulerSnapshotService) runPenaltyRecoveryWorker(interval time.Duration) {
	// 扫描 temp_unschedulable_until <= now 的账号
	// 按 accountID + until 去重
	// 触发 account_changed / 快照刷新
}
```

- [ ] **Step 4: 跑恢复测试确认 GREEN**

Run: `C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe test -tags unit ./internal/service -run TestPenaltyRecoveryScan_RefreshesExpiredTempUnschedAccountOnce -count=1`

Expected: PASS。

- [ ] **Step 5: 提交这一小步**

```bash
git add backend/internal/service/scheduler_snapshot_service.go backend/internal/service/scheduler_snapshot_penalty_recovery_test.go
git commit -m "fix(openai): 增加惩罚到期后的快照恢复"
```

## Task 5: 做整体验证与发布演练脚本化检查

**Files:**
- Modify: `backend/internal/handler/openai_gateway_handler_test.go`
- Modify: `backend/internal/service/openai_retry_penalty_test.go`
- Modify: `backend/internal/service/scheduler_snapshot_penalty_recovery_test.go`
- Modify: `AGENTS.md`（仅在实现过程中发现新的稳定部署经验时才改）

- [ ] **Step 1: 增加关键回归测试**

```go
func TestOpenAIResponsesFailover_DoesNotBreakStickyOrPreviousResponsePriority(t *testing.T) {}
func TestOpenAIRetryPenaltyTracker_SuccessOfAOnlyClearsA(t *testing.T) {}
func TestOpenAIResponsesFailover_DoesNotCountPreviousResponseNotFoundAsPenalty(t *testing.T) {}
```

- [ ] **Step 2: 运行完整相关测试集**

Run: `C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe test -tags unit ./internal/service -run "TestOpenAIRetryPenaltyTracker_.*|TestPenaltyRecoveryScan_.*|TestSelectByLoadBalance_Exhausted.*Reserve|TestShouldUseReserveForExhaustedOverflow.*" -count=1`

Expected: PASS。

- [ ] **Step 3: 运行 handler / build / diff 基线**

Run:

```bash
C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe test ./internal/handler -count=1
C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe build ./cmd/server
git diff --check
```

Expected: 全部通过。

- [ ] **Step 4: 计划执行后的发版检查清单**

```text
1. GOOS=linux GOARCH=amd64 CGO_ENABLED=0 交叉编译 embed 二进制
2. 上传候选二进制到 /tmp
3. 127.0.0.1:18081 smoke，验证 /health
4. 如条件允许，做一次真实 OpenAI failover 请求链路验证：
   - A 连续失败两次后切到 B
   - 账号触发 temp_unsched 惩罚后，在窗口内不再命中
5. 提升正式版本并检查 /health
```

- [ ] **Step 5: 提交最终实现**

```bash
git add backend/internal/handler/openai_gateway_handler_test.go backend/internal/service/openai_retry_penalty_test.go backend/internal/service/scheduler_snapshot_penalty_recovery_test.go
git commit -m "test(openai): 补齐重试换号与惩罚回归覆盖"
```
