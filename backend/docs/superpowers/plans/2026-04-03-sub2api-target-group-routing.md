# sub2api Target-Group Routing Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在 `sub2api` 的 OpenAI/Codex 网关链路中引入 `active` / `exhausted` 目标组路由、`-Sys` 模型别名与最小 continuation 注入，同时保住现有 `previous_response_id`、sticky、failover 与负载均衡语义。

**Architecture:** 在 `OpenAIGatewayHandler.Responses()` 入口完成请求分类和 `-Sys` 预处理，再把 `TargetGroup` 传给 scheduler。scheduler 的 `previous_response_id`、sticky、load-balance` 三层统一使用 `MatchesTargetGroup()` 与 `IsSchedulableForTargetGroup()`，其中 exhausted 组跳过 `RateLimitResetAt` 检查但仍保留过期、临时不可调度和过载保护。`/v1/models` 保持在响应整形层追加 `-Sys` 别名，避免污染账号模型聚合逻辑。

**Tech Stack:** Go, Gin handlers, existing OpenAI gateway/service scheduler, `gjson`, `go test`, `gofmt`.

---

## 文件结构

- `internal/service/account.go`
  增加 `IsExhausted()`、`MatchesTargetGroup()`、`IsSchedulableForTargetGroup()`，并复用现有 `getExtraFloat64()` / `IsQuotaExceeded()`。
- `internal/service/account_target_group_test.go`（新建）
  锁定 OAuth exhausted 判定、API Key quota exhausted 判定，以及 exhausted 组选号时跳过 `RateLimitResetAt` 的语义。
- `internal/service/openai_account_scheduler.go`
  定义 `AccountTargetGroup`、扩展 `OpenAIAccountScheduleRequest`、让 previous-response / sticky / load-balance 三层都感知目标组。
- `internal/service/openai_gateway_service.go`
  扩展 `SelectAccountWithScheduler(...)` 签名，并把 snapshot / DB recheck 两层辅助函数也改成按目标组判断，避免 exhausted 账号在二次校验阶段又被普通 `IsSchedulable()` 挡掉。
- `internal/service/openai_ws_forwarder.go`
  扩展 `SelectAccountByPreviousResponseID(...)` 签名与逻辑，使 previous-response 命中账号在“仅目标组不匹配”时只跳过、不删绑定。
- `internal/service/openai_account_scheduler_test.go`
  追加 sticky 命中目标组不匹配时保留绑定、load-balance 允许 exhausted+rate-limited 账号的测试。
- `internal/service/openai_ws_account_sticky_test.go`
  追加 previous-response 命中目标组不匹配时保留绑定的测试。
- `internal/service/openai_tool_continuation.go`
  在现有文件中新增 `GetRequestTargetGroup()`、`NeedsSysToolContinuation()`、`AppendMinimalSysToolContinuation()`，不要新建重复文件。
- `internal/service/openai_tool_continuation_test.go`
  在现有测试文件中补充目标组分类、`-Sys` 续链触发条件、最小 dummy pair 结构测试。
- `internal/service/openai_model_mapping.go`
  增加 `IsSysModel()`、`StripSysSuffix()`。
- `internal/service/openai_model_mapping_test.go`
  追加 `-Sys` helper 测试。
- `internal/handler/openai_gateway_handler.go`
  在 `Responses()` 中接入 `TargetGroup` 分类与 `-Sys` 预处理；同时把 `Messages()`、`ResponsesWebSocket()` 这两个已有调度入口改成显式传 `TargetGroupAny`。
- `internal/handler/openai_chat_completions.go`
  把两处 `SelectAccountWithScheduler(...)` 调用都补上 `TargetGroupAny`。
- `internal/handler/openai_gateway_handler_test.go`
  通过新增纯 helper 测试，锁定 `Responses()` 预处理逻辑，而不是假设不存在的 mock service。
- `internal/handler/gateway_handler.go`
  在 `/v1/models` 的 whitelist 分支（`[]claude.Model`）和 fallback 分支（`[]openai.Model`）都追加 `-Sys` 别名。
- `internal/handler/gateway_handler_models_test.go`（新建）
  锁定两种模型结构上的别名追加、大小写去重、紧邻排序。

## 实施顺序

先固化账号与 `TargetGroup` 语义，再打通 scheduler/service 的三层调度与二次校验，随后补 `-Sys`/continuation helper，最后把 `Responses()` 和 `/v1/models` 接上并做整仓回归。这样每一步都能独立编译、独立跑测试，不会出现“中途签名改了一半导致整个仓库都不能跑”的状态。

### Task 1: 固定 `TargetGroup` 与账号判定语义

**Files:**
- Create: `internal/service/account_target_group_test.go`
- Modify: `internal/service/account.go`
- Modify: `internal/service/openai_account_scheduler.go`

- [ ] **Step 1: 先写失败测试，锁定 exhausted 判定与按组可调度语义**

```go
package service

import (
    "testing"
    "time"

    "github.com/stretchr/testify/require"
)

func TestAccountIsExhausted(t *testing.T) {
    t.Run("oauth_7d_snapshot", func(t *testing.T) {
        account := &Account{
            Platform: PlatformOpenAI,
            Type:     AccountTypeOAuth,
            Extra: map[string]any{
                "codex_7d_used_percent": 100.0,
            },
        }
        require.True(t, account.IsExhausted())
    })

    t.Run("oauth_primary_snapshot", func(t *testing.T) {
        account := &Account{
            Platform: PlatformOpenAI,
            Type:     AccountTypeOAuth,
            Extra: map[string]any{
                "codex_primary_used_percent": 100.0,
            },
        }
        require.True(t, account.IsExhausted())
    })

    t.Run("api_key_quota", func(t *testing.T) {
        account := &Account{
            Platform: PlatformOpenAI,
            Type:     AccountTypeAPIKey,
            Extra: map[string]any{
                "quota_limit": 100.0,
                "quota_used":  100.0,
            },
        }
        require.True(t, account.IsExhausted())
    })

    t.Run("below_threshold_stays_active", func(t *testing.T) {
        account := &Account{
            Platform: PlatformOpenAI,
            Type:     AccountTypeOAuth,
            Extra: map[string]any{
                "codex_7d_used_percent":      99.0,
                "codex_primary_used_percent": 20.0,
            },
        }
        require.False(t, account.IsExhausted())
    })
}

func TestAccountIsSchedulableForTargetGroup(t *testing.T) {
    rateLimitedUntil := time.Now().Add(30 * time.Minute)
    tempBlockedUntil := time.Now().Add(30 * time.Minute)

    exhausted := &Account{
        Platform:    PlatformOpenAI,
        Type:        AccountTypeOAuth,
        Status:      StatusActive,
        Schedulable: true,
        Extra: map[string]any{
            "codex_7d_used_percent": 100.0,
        },
        RateLimitResetAt: &rateLimitedUntil,
    }
    require.True(t, exhausted.IsSchedulableForTargetGroup(TargetGroupExhausted))
    require.False(t, exhausted.IsSchedulableForTargetGroup(TargetGroupActive))

    blocked := &Account{
        Platform:               PlatformOpenAI,
        Type:                   AccountTypeOAuth,
        Status:                 StatusActive,
        Schedulable:            true,
        TempUnschedulableUntil: &tempBlockedUntil,
        Extra: map[string]any{
            "codex_primary_used_percent": 100.0,
        },
    }
    require.False(t, blocked.IsSchedulableForTargetGroup(TargetGroupExhausted))
}
```

- [ ] **Step 2: 运行失败测试，确认当前代码还没有这些语义**

Run: `go test ./internal/service -run "TestAccount(IsExhausted|IsSchedulableForTargetGroup)" -count=1`
Expected: FAIL，报 `IsExhausted` / `IsSchedulableForTargetGroup` / `TargetGroupExhausted` 未定义，或 `RateLimitResetAt` 仍会挡住 exhausted 组。

- [ ] **Step 3: 在 scheduler 文件中加入 `AccountTargetGroup` 与请求字段，保持现有类型不变**

```go
type AccountTargetGroup string

const (
    TargetGroupAny       AccountTargetGroup = ""
    TargetGroupActive    AccountTargetGroup = "active"
    TargetGroupExhausted AccountTargetGroup = "exhausted"
)

func normalizeTargetGroup(group AccountTargetGroup) AccountTargetGroup {
    switch group {
    case TargetGroupActive, TargetGroupExhausted:
        return group
    default:
        return TargetGroupAny
    }
}

type OpenAIAccountScheduleRequest struct {
    GroupID            *int64
    SessionHash        string
    StickyAccountID    int64
    PreviousResponseID string
    RequestedModel     string
    TargetGroup        AccountTargetGroup
    RequiredTransport  OpenAIUpstreamTransport
    ExcludedIDs        map[int64]struct{}
}
```

- [ ] **Step 4: 在 `account.go` 实现 exhausted 判定、目标组匹配和按组可调度检查**

```go
func (a *Account) IsExhausted() bool {
    if a == nil || !a.IsOpenAI() {
        return false
    }
    if a.IsOAuth() {
        return a.getExtraFloat64("codex_7d_used_percent") >= 100 ||
            a.getExtraFloat64("codex_primary_used_percent") >= 100
    }
    return a.IsQuotaExceeded()
}

func (a *Account) MatchesTargetGroup(group AccountTargetGroup) bool {
    switch normalizeTargetGroup(group) {
    case TargetGroupExhausted:
        return a.IsExhausted()
    case TargetGroupActive:
        return !a.IsExhausted()
    default:
        return true
    }
}

func (a *Account) IsSchedulableForTargetGroup(group AccountTargetGroup) bool {
    normalized := normalizeTargetGroup(group)
    if normalized != TargetGroupExhausted {
        return a.IsSchedulable() && a.MatchesTargetGroup(normalized)
    }

    if a == nil || !a.IsActive() || !a.Schedulable {
        return false
    }
    now := time.Now()
    if a.AutoPauseOnExpired && a.ExpiresAt != nil && !now.Before(*a.ExpiresAt) {
        return false
    }
    if a.OverloadUntil != nil && now.Before(*a.OverloadUntil) {
        return false
    }
    if a.TempUnschedulableUntil != nil && now.Before(*a.TempUnschedulableUntil) {
        return false
    }
    return a.MatchesTargetGroup(normalized)
}
```

- [ ] **Step 5: 重新运行账号层测试，确认基础语义稳定**

Run: `go test ./internal/service -run "TestAccount(IsExhausted|IsSchedulableForTargetGroup)" -count=1`
Expected: PASS，覆盖 OAuth exhausted、API Key quota exhausted、exhausted 组忽略 `RateLimitResetAt`、`TempUnschedulableUntil` 仍然阻断四个关键语义。

- [ ] **Step 6: 提交这一层变更**

```bash
git add internal/service/account.go internal/service/openai_account_scheduler.go internal/service/account_target_group_test.go
git commit -m "feat(service): 新增账号目标组判定语义"
```

### Task 2: 让 scheduler、service helper 与全部调用点感知 `TargetGroup`

**Files:**
- Modify: `internal/service/openai_account_scheduler.go`
- Modify: `internal/service/openai_gateway_service.go`
- Modify: `internal/service/openai_ws_forwarder.go`
- Modify: `internal/service/openai_account_scheduler_test.go`
- Modify: `internal/service/openai_ws_account_sticky_test.go`
- Modify: `internal/handler/openai_gateway_handler.go`
- Modify: `internal/handler/openai_chat_completions.go`

- [ ] **Step 1: 先补失败测试，锁定 sticky / previous-response / load-balance 的目标组行为**

```go
func TestOpenAIGatewayService_SelectAccountWithScheduler_SessionStickyTargetGroupMismatchKeepsBinding(t *testing.T) {
    ctx := context.Background()
    groupID := int64(10110)
    rateLimitedUntil := time.Now().Add(30 * time.Minute)

    active := Account{ID: 41001, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 0}
    exhausted := Account{
        ID:               41002,
        Platform:         PlatformOpenAI,
        Type:             AccountTypeOAuth,
        Status:           StatusActive,
        Schedulable:      true,
        Concurrency:      1,
        Priority:         5,
        RateLimitResetAt: &rateLimitedUntil,
        Extra:            map[string]any{"codex_7d_used_percent": 100.0},
    }

    cache := &stubGatewayCache{sessionBindings: map[string]int64{"openai:session_hash_target_group": active.ID}}
    svc := &OpenAIGatewayService{
        accountRepo:        stubOpenAIAccountRepo{accounts: []Account{active, exhausted}},
        cache:              cache,
        cfg:                &config.Config{},
        concurrencyService: NewConcurrencyService(stubConcurrencyCache{}),
    }

    selection, decision, err := svc.SelectAccountWithScheduler(ctx, &groupID, "", "session_hash_target_group", "gpt-5.1", TargetGroupExhausted, nil, OpenAIUpstreamTransportAny)
    require.NoError(t, err)
    require.NotNil(t, selection)
    require.Equal(t, exhausted.ID, selection.Account.ID)
    require.Equal(t, openAIAccountScheduleLayerLoadBalance, decision.Layer)
    require.Equal(t, active.ID, cache.sessionBindings["openai:session_hash_target_group"])
}

func TestOpenAIGatewayService_SelectAccountWithScheduler_ExhaustedRateLimitedStickyKeepsBinding(t *testing.T) {
    ctx := context.Background()
    groupID := int64(10113)
    rateLimitedUntil := time.Now().Add(30 * time.Minute)

    exhausted := Account{
        ID:               44001,
        Platform:         PlatformOpenAI,
        Type:             AccountTypeOAuth,
        Status:           StatusActive,
        Schedulable:      true,
        Concurrency:      1,
        Priority:         0,
        RateLimitResetAt: &rateLimitedUntil,
        Extra:            map[string]any{"codex_7d_used_percent": 100.0},
    }

    cache := &stubGatewayCache{sessionBindings: map[string]int64{"openai:session_hash_exhausted": exhausted.ID}}
    svc := &OpenAIGatewayService{
        accountRepo:        stubOpenAIAccountRepo{accounts: []Account{exhausted}},
        cache:              cache,
        cfg:                &config.Config{},
        concurrencyService: NewConcurrencyService(stubConcurrencyCache{}),
    }

    selection, decision, err := svc.SelectAccountWithScheduler(ctx, &groupID, "", "session_hash_exhausted", "gpt-5.1", TargetGroupExhausted, nil, OpenAIUpstreamTransportAny)
    require.NoError(t, err)
    require.NotNil(t, selection)
    require.Equal(t, exhausted.ID, selection.Account.ID)
    require.Equal(t, openAIAccountScheduleLayerSessionSticky, decision.Layer)
    require.Equal(t, exhausted.ID, cache.sessionBindings["openai:session_hash_exhausted"])
}

func TestOpenAIGatewayService_SelectAccountWithScheduler_SessionStickyUnschedulableClearsBinding(t *testing.T) {
    ctx := context.Background()
    groupID := int64(10112)
    tempBlockedUntil := time.Now().Add(30 * time.Minute)

    blocked := Account{
        ID:                    43001,
        Platform:              PlatformOpenAI,
        Type:                  AccountTypeOAuth,
        Status:                StatusActive,
        Schedulable:           true,
        Concurrency:           1,
        Priority:              0,
        TempUnschedulableUntil: &tempBlockedUntil,
    }
    backup := Account{ID: 43002, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 5}

    cache := &stubGatewayCache{sessionBindings: map[string]int64{"openai:session_hash_cleanup": blocked.ID}}
    svc := &OpenAIGatewayService{
        accountRepo:        stubOpenAIAccountRepo{accounts: []Account{blocked, backup}},
        cache:              cache,
        cfg:                &config.Config{},
        concurrencyService: NewConcurrencyService(stubConcurrencyCache{}),
    }

    selection, decision, err := svc.SelectAccountWithScheduler(ctx, &groupID, "", "session_hash_cleanup", "gpt-5.1", TargetGroupActive, nil, OpenAIUpstreamTransportAny)
    require.NoError(t, err)
    require.NotNil(t, selection)
    require.Equal(t, backup.ID, selection.Account.ID)
    require.Equal(t, openAIAccountScheduleLayerLoadBalance, decision.Layer)
    require.Equal(t, 1, cache.deletedSessions["openai:session_hash_cleanup"])
}

func TestOpenAIGatewayService_SelectAccountWithScheduler_LoadBalanceAllowsRateLimitedExhaustedAccount(t *testing.T) {
    ctx := context.Background()
    groupID := int64(10111)
    rateLimitedUntil := time.Now().Add(30 * time.Minute)

    exhausted := Account{
        ID:               42001,
        Platform:         PlatformOpenAI,
        Type:             AccountTypeOAuth,
        Status:           StatusActive,
        Schedulable:      true,
        Concurrency:      1,
        Priority:         0,
        RateLimitResetAt: &rateLimitedUntil,
        Extra:            map[string]any{"codex_primary_used_percent": 100.0},
    }

    svc := &OpenAIGatewayService{
        accountRepo:        stubOpenAIAccountRepo{accounts: []Account{exhausted}},
        cfg:                &config.Config{},
        concurrencyService: NewConcurrencyService(stubConcurrencyCache{}),
    }

    selection, decision, err := svc.SelectAccountWithScheduler(ctx, &groupID, "", "", "gpt-5.1", TargetGroupExhausted, nil, OpenAIUpstreamTransportAny)
    require.NoError(t, err)
    require.NotNil(t, selection)
    require.Equal(t, exhausted.ID, selection.Account.ID)
    require.Equal(t, openAIAccountScheduleLayerLoadBalance, decision.Layer)
}

func TestOpenAIGatewayService_SelectAccountByPreviousResponseID_TargetGroupMismatchKeepsBinding(t *testing.T) {
    ctx := context.Background()
    groupID := int64(25)

    account := Account{
        ID:          31,
        Platform:    PlatformOpenAI,
        Type:        AccountTypeAPIKey,
        Status:      StatusActive,
        Schedulable: true,
        Concurrency: 1,
        Extra: map[string]any{
            "openai_apikey_responses_websockets_v2_enabled": true,
        },
    }

    cache := &stubGatewayCache{}
    store := NewOpenAIWSStateStore(cache)
    svc := &OpenAIGatewayService{
        accountRepo:        stubOpenAIAccountRepo{accounts: []Account{account}},
        cache:              cache,
        cfg:                newOpenAIWSV2TestConfig(),
        concurrencyService: NewConcurrencyService(stubConcurrencyCache{}),
        openaiWSStateStore: store,
    }

    require.NoError(t, store.BindResponseAccount(ctx, groupID, "resp_prev_target_group", account.ID, time.Hour))

    selection, err := svc.SelectAccountByPreviousResponseID(ctx, &groupID, "resp_prev_target_group", "gpt-5.1", TargetGroupExhausted, nil)
    require.NoError(t, err)
    require.Nil(t, selection)

    boundAccountID, getErr := store.GetResponseAccount(ctx, groupID, "resp_prev_target_group")
    require.NoError(t, getErr)
    require.Equal(t, account.ID, boundAccountID)
}

func TestOpenAIGatewayService_SelectAccountByPreviousResponseID_UnschedulableClearsBinding(t *testing.T) {
    ctx := context.Background()
    groupID := int64(26)
    tempBlockedUntil := time.Now().Add(30 * time.Minute)

    account := Account{
        ID:                    32,
        Platform:              PlatformOpenAI,
        Type:                  AccountTypeAPIKey,
        Status:                StatusActive,
        Schedulable:           true,
        Concurrency:           1,
        TempUnschedulableUntil: &tempBlockedUntil,
        Extra: map[string]any{
            "openai_apikey_responses_websockets_v2_enabled": true,
        },
    }

    cache := &stubGatewayCache{}
    store := NewOpenAIWSStateStore(cache)
    svc := &OpenAIGatewayService{
        accountRepo:        stubOpenAIAccountRepo{accounts: []Account{account}},
        cache:              cache,
        cfg:                newOpenAIWSV2TestConfig(),
        concurrencyService: NewConcurrencyService(stubConcurrencyCache{}),
        openaiWSStateStore: store,
    }

    require.NoError(t, store.BindResponseAccount(ctx, groupID, "resp_prev_cleanup", account.ID, time.Hour))

    selection, err := svc.SelectAccountByPreviousResponseID(ctx, &groupID, "resp_prev_cleanup", "gpt-5.1", TargetGroupActive, nil)
    require.NoError(t, err)
    require.Nil(t, selection)

    boundAccountID, getErr := store.GetResponseAccount(ctx, groupID, "resp_prev_cleanup")
    require.NoError(t, getErr)
    require.Zero(t, boundAccountID)
}
```

- [ ] **Step 2: 运行定向失败测试，确认当前链路会错误清缓存或错误过滤 exhausted**

Run: `go test ./internal/service -run "TestOpenAIGatewayService_(SelectAccountWithScheduler_SessionStickyTargetGroupMismatchKeepsBinding|SelectAccountWithScheduler_ExhaustedRateLimitedStickyKeepsBinding|SelectAccountWithScheduler_SessionStickyUnschedulableClearsBinding|SelectAccountWithScheduler_LoadBalanceAllowsRateLimitedExhaustedAccount|SelectAccountByPreviousResponseID_TargetGroupMismatchKeepsBinding|SelectAccountByPreviousResponseID_UnschedulableClearsBinding)" -count=1`
Expected: FAIL，原因应落在三类问题之一：`SelectAccountWithScheduler(...)`/`SelectAccountByPreviousResponseID(...)` 签名还没有 `TargetGroup`，sticky / previous-response 在目标组不匹配时提前删绑定，或 exhausted 账号仍被普通 `IsSchedulable()` 挡住。

- [ ] **Step 3: 扩展请求结构、service 包装方法和 previous-response 签名**

```go
func (s *OpenAIGatewayService) SelectAccountWithScheduler(
    ctx context.Context,
    groupID *int64,
    previousResponseID string,
    sessionHash string,
    requestedModel string,
    targetGroup AccountTargetGroup,
    excludedIDs map[int64]struct{},
    requiredTransport OpenAIUpstreamTransport,
) (*AccountSelectionResult, OpenAIAccountScheduleDecision, error) {
    decision := OpenAIAccountScheduleDecision{}
    scheduler := s.getOpenAIAccountScheduler()
    if scheduler == nil {
        selection, err := s.SelectAccountWithLoadAwareness(ctx, groupID, sessionHash, requestedModel, excludedIDs)
        decision.Layer = openAIAccountScheduleLayerLoadBalance
        return selection, decision, err
    }

    var stickyAccountID int64
    if sessionHash != "" && s.cache != nil {
        if accountID, err := s.getStickySessionAccountID(ctx, groupID, sessionHash); err == nil && accountID > 0 {
            stickyAccountID = accountID
        }
    }

    return scheduler.Select(ctx, OpenAIAccountScheduleRequest{
        GroupID:            groupID,
        SessionHash:        sessionHash,
        StickyAccountID:    stickyAccountID,
        PreviousResponseID: previousResponseID,
        RequestedModel:     requestedModel,
        TargetGroup:        normalizeTargetGroup(targetGroup),
        RequiredTransport:  requiredTransport,
        ExcludedIDs:        excludedIDs,
    })
}

func (s *OpenAIGatewayService) SelectAccountByPreviousResponseID(
    ctx context.Context,
    groupID *int64,
    previousResponseID string,
    requestedModel string,
    targetGroup AccountTargetGroup,
    excludedIDs map[int64]struct{},
) (*AccountSelectionResult, error)
```

- [ ] **Step 4: 修改 sticky / previous-response / load-balance 三层，并把 snapshot / DB recheck 也改成按组判断**

```go
func (s *OpenAIGatewayService) resolveFreshSchedulableOpenAIAccountForTargetGroup(
    ctx context.Context,
    account *Account,
    requestedModel string,
    targetGroup AccountTargetGroup,
) *Account {
    if account == nil {
        return nil
    }

    fresh := account
    if s.schedulerSnapshot != nil {
        current, err := s.getSchedulableAccount(ctx, account.ID)
        if err != nil || current == nil {
            return nil
        }
        fresh = current
    }

    if !fresh.IsSchedulableForTargetGroup(targetGroup) || !fresh.IsOpenAI() {
        return nil
    }
    if requestedModel != "" && !fresh.IsModelSupported(requestedModel) {
        return nil
    }
    return fresh
}

func (s *OpenAIGatewayService) recheckSelectedOpenAIAccountFromDBForTargetGroup(
    ctx context.Context,
    account *Account,
    requestedModel string,
    targetGroup AccountTargetGroup,
) *Account {
    if account == nil {
        return nil
    }
    if s.schedulerSnapshot == nil || s.accountRepo == nil {
        if account.IsSchedulableForTargetGroup(targetGroup) && account.IsOpenAI() {
            return account
        }
        return nil
    }

    latest, err := s.accountRepo.GetByID(ctx, account.ID)
    if err != nil || latest == nil {
        return nil
    }
    syncOpenAICodexRateLimitFromExtra(ctx, s.accountRepo, latest, time.Now())
    if !latest.IsSchedulableForTargetGroup(targetGroup) || !latest.IsOpenAI() {
        return nil
    }
    if requestedModel != "" && !latest.IsModelSupported(requestedModel) {
        return nil
    }
    return latest
}
```

```go
func shouldClearStickyBindingForTargetGroup(account *Account, requestedModel string, targetGroup AccountTargetGroup) bool {
    if account == nil {
        return false
    }
    if account.Status == StatusError || account.Status == StatusDisabled || !account.Schedulable {
        return true
    }
    if account.TempUnschedulableUntil != nil && time.Now().Before(*account.TempUnschedulableUntil) {
        return true
    }
    if normalizeTargetGroup(targetGroup) == TargetGroupExhausted {
        return false
    }
    return account.GetRateLimitRemainingTimeWithContext(context.Background(), requestedModel) > 0
}
```

```go
if !account.IsOpenAI() {
    _ = s.service.deleteStickySessionAccountID(ctx, req.GroupID, sessionHash)
    return nil, nil
}
if !account.MatchesTargetGroup(req.TargetGroup) {
    return nil, nil
}
if shouldClearStickyBindingForTargetGroup(account, req.RequestedModel, req.TargetGroup) {
    _ = s.service.deleteStickySessionAccountID(ctx, req.GroupID, sessionHash)
    return nil, nil
}
if !account.IsSchedulableForTargetGroup(req.TargetGroup) {
    _ = s.service.deleteStickySessionAccountID(ctx, req.GroupID, sessionHash)
    return nil, nil
}
```

```go
if !account.IsOpenAI() {
    _ = store.DeleteResponseAccount(ctx, derefGroupID(groupID), responseID)
    return nil, nil
}
if !account.MatchesTargetGroup(targetGroup) {
    return nil, nil
}
if shouldClearStickyBindingForTargetGroup(account, requestedModel, targetGroup) {
    _ = store.DeleteResponseAccount(ctx, derefGroupID(groupID), responseID)
    return nil, nil
}
if !account.IsSchedulableForTargetGroup(targetGroup) {
    _ = store.DeleteResponseAccount(ctx, derefGroupID(groupID), responseID)
    return nil, nil
}
```

```go
for i := range accounts {
    account := &accounts[i]
    if req.ExcludedIDs != nil {
        if _, excluded := req.ExcludedIDs[account.ID]; excluded {
            continue
        }
    }
    if !account.IsSchedulableForTargetGroup(req.TargetGroup) || !account.IsOpenAI() {
        continue
    }
    if req.RequestedModel != "" && !account.IsModelSupported(req.RequestedModel) {
        continue
    }
    if !s.isAccountTransportCompatible(account, req.RequiredTransport) {
        continue
    }
    filtered = append(filtered, account)
}
```

- [ ] **Step 5: 一次性改完全部 `SelectAccountWithScheduler(...)` 调用点，先统一传 `TargetGroupAny` 保持行为不变**

```go
selection, scheduleDecision, err := h.gatewayService.SelectAccountWithScheduler(
    c.Request.Context(),
    apiKey.GroupID,
    previousResponseID,
    sessionHash,
    reqModel,
    service.TargetGroupAny,
    failedAccountIDs,
    service.OpenAIUpstreamTransportAny,
)
```

```go
selection, scheduleDecision, err := h.gatewayService.SelectAccountWithScheduler(
    ctx,
    apiKey.GroupID,
    previousResponseID,
    sessionHash,
    reqModel,
    service.TargetGroupAny,
    nil,
    service.OpenAIUpstreamTransportResponsesWebsocketV2,
)
```

同一任务内把以下调用一并改完，避免仓库中途无法编译：
- `internal/handler/openai_gateway_handler.go` 的 `Responses()` 主调度调用
- `internal/handler/openai_gateway_handler.go` 的 `Messages()` 常规调度调用
- `internal/handler/openai_gateway_handler.go` 的 `Messages()` 默认模型回退调用
- `internal/handler/openai_gateway_handler.go` 的 `ResponsesWebSocket()` 调度调用
- `internal/handler/openai_chat_completions.go` 的两处调度调用

- [ ] **Step 6: 重新运行 service 定向测试，确认三层路径与调用签名都稳定**

Run: `go test ./internal/service -run "TestOpenAIGatewayService_(SelectAccountWithScheduler_SessionStickyTargetGroupMismatchKeepsBinding|SelectAccountWithScheduler_ExhaustedRateLimitedStickyKeepsBinding|SelectAccountWithScheduler_SessionStickyUnschedulableClearsBinding|SelectAccountWithScheduler_LoadBalanceAllowsRateLimitedExhaustedAccount|SelectAccountByPreviousResponseID_TargetGroupMismatchKeepsBinding|SelectAccountByPreviousResponseID_UnschedulableClearsBinding)" -count=1`
Expected: PASS，目标组不匹配只跳过、真实不可调度才清缓存，且 exhausted 组可以选中 rate-limited exhausted 账号。

- [ ] **Step 7: 提交调度链路变更**

```bash
git add internal/service/openai_account_scheduler.go internal/service/openai_gateway_service.go internal/service/openai_ws_forwarder.go internal/service/openai_account_scheduler_test.go internal/service/openai_ws_account_sticky_test.go internal/handler/openai_gateway_handler.go internal/handler/openai_chat_completions.go
git commit -m "feat(scheduler): 让目标组约束贯穿调度链路"
```

### Task 3: 在现有 helper 文件里补齐 `-Sys` 与 continuation 规则

**Files:**
- Modify: `internal/service/openai_tool_continuation.go`
- Modify: `internal/service/openai_tool_continuation_test.go`
- Modify: `internal/service/openai_model_mapping.go`
- Modify: `internal/service/openai_model_mapping_test.go`

- [ ] **Step 1: 先写失败测试，锁定 `-Sys` helper、目标组分类和最小 continuation 结构**

```go
func TestIsSysModel(t *testing.T) {
    require.True(t, IsSysModel("gpt-5.4-Sys"))
    require.True(t, IsSysModel("GPT-5.4-sYs"))
    require.False(t, IsSysModel("gpt-5.4"))
}

func TestStripSysSuffix(t *testing.T) {
    require.Equal(t, "gpt-5.4", StripSysSuffix("gpt-5.4-Sys"))
    require.Equal(t, "gpt-5.4", StripSysSuffix("gpt-5.4"))
}

func TestGetRequestTargetGroup(t *testing.T) {
    exhaustedReq := map[string]any{
        "input": []any{map[string]any{"type": "function_call_output", "call_id": "call_1", "output": "ok"}},
    }
    activeReq := map[string]any{
        "input": []any{map[string]any{"type": "message", "role": "user"}},
    }

    require.Equal(t, TargetGroupExhausted, GetRequestTargetGroup(exhaustedReq))
    require.Equal(t, TargetGroupActive, GetRequestTargetGroup(activeReq))
}

func TestNeedsSysToolContinuation(t *testing.T) {
    require.True(t, NeedsSysToolContinuation(map[string]any{"input": []any{map[string]any{"type": "message", "role": "user"}}}))
    require.True(t, NeedsSysToolContinuation(map[string]any{"input": []any{map[string]any{"type": "item_reference", "id": "call_1"}}}))
    require.False(t, NeedsSysToolContinuation(map[string]any{"input": []any{map[string]any{"type": "function_call_output", "call_id": "call_1", "output": "ok"}}}))
}

func TestAppendMinimalSysToolContinuation(t *testing.T) {
    reqBody := map[string]any{
        "input": []any{map[string]any{"type": "message", "role": "user"}},
    }

    AppendMinimalSysToolContinuation(reqBody)

    input := reqBody["input"].([]any)
    require.Len(t, input, 3)
    require.Equal(t, "tool_call", input[1].(map[string]any)["type"])
    require.Equal(t, "sys_dummy", input[1].(map[string]any)["call_id"])
    require.Equal(t, "sys_status", input[1].(map[string]any)["name"])
    require.Equal(t, "function_call_output", input[2].(map[string]any)["type"])
    require.Equal(t, "sys_dummy", input[2].(map[string]any)["call_id"])
    require.Equal(t, "ready", input[2].(map[string]any)["output"])
}
```

- [ ] **Step 2: 运行 helper 失败测试，确认当前 helper 还不完整**

Run: `go test ./internal/service -run "Test(IsSysModel|StripSysSuffix|GetRequestTargetGroup|NeedsSysToolContinuation|AppendMinimalSysToolContinuation)" -count=1`
Expected: FAIL，报 `-Sys` helper 未定义，或 continuation 结构仍然不是 `tool_call + function_call_output`。

- [ ] **Step 3: 在 `openai_model_mapping.go` 中加入 `-Sys` 识别与去后缀 helper**

```go
func IsSysModel(model string) bool {
    return strings.HasSuffix(strings.ToLower(strings.TrimSpace(model)), "-sys")
}

func StripSysSuffix(model string) string {
    trimmed := strings.TrimSpace(model)
    if !IsSysModel(trimmed) {
        return trimmed
    }
    return trimmed[:len(trimmed)-4]
}
```

- [ ] **Step 4: 在现有 `openai_tool_continuation.go` 中增量补 helper，而不是新建重复文件**

```go
func GetRequestTargetGroup(reqBody map[string]any) AccountTargetGroup {
    input, _ := reqBody["input"].([]any)
    if len(input) == 0 {
        return TargetGroupActive
    }
    last, _ := input[len(input)-1].(map[string]any)
    itemType, _ := last["type"].(string)
    if strings.EqualFold(strings.TrimSpace(itemType), "function_call_output") {
        return TargetGroupExhausted
    }
    return TargetGroupActive
}

func NeedsSysToolContinuation(reqBody map[string]any) bool {
    input, _ := reqBody["input"].([]any)
    if len(input) == 0 {
        return false
    }
    last, _ := input[len(input)-1].(map[string]any)
    itemType, _ := last["type"].(string)
    switch strings.TrimSpace(itemType) {
    case "item_reference":
        return true
    case "message":
        role, _ := last["role"].(string)
        return strings.EqualFold(strings.TrimSpace(role), "user")
    default:
        return false
    }
}

func AppendMinimalSysToolContinuation(reqBody map[string]any) {
    items, _ := reqBody["input"].([]any)
    callID := "sys_dummy"

    reqBody["input"] = append(items,
        map[string]any{
            "type":      "tool_call",
            "call_id":   callID,
            "name":      "sys_status",
            "arguments": "{}",
        },
        map[string]any{
            "type":    "function_call_output",
            "call_id": callID,
            "output":  "ready",
        },
    )
}
```

- [ ] **Step 5: 重新运行 helper 测试，确认 `-Sys` 与 continuation 规则被锁定**

Run: `go test ./internal/service -run "Test(IsSysModel|StripSysSuffix|GetRequestTargetGroup|NeedsSysToolContinuation|AppendMinimalSysToolContinuation)" -count=1`
Expected: PASS，覆盖大小写 `-Sys`、`function_call_output` 归 exhausted、用户消息与 `item_reference` 触发 continuation、dummy pair 为 `tool_call + function_call_output` 且 `output == "ready"`。

- [ ] **Step 6: 提交 helper 层变更**

```bash
git add internal/service/openai_tool_continuation.go internal/service/openai_tool_continuation_test.go internal/service/openai_model_mapping.go internal/service/openai_model_mapping_test.go
git commit -m "feat(openai): 新增 Sys 模型续链 helper"
```

### Task 4: 把真实 `TargetGroup` 与 `-Sys` 预处理接入 `Responses()`

**Files:**
- Modify: `internal/handler/openai_gateway_handler.go`
- Modify: `internal/handler/openai_gateway_handler_test.go`

- [ ] **Step 1: 为 `Responses()` 新增纯 helper 测试，避免假设不存在的 mock service**

```go
func TestPrepareResponsesRequestForScheduling_FunctionCallOutputUsesExhaustedGroup(t *testing.T) {
    gin.SetMode(gin.TestMode)
    rec := httptest.NewRecorder()
    c, _ := gin.CreateTestContext(rec)

    body := []byte(`{"model":"gpt-5.4","input":[{"type":"function_call_output","call_id":"call_1","output":"ok"}]}`)

    patchedBody, patchedModel, targetGroup, err := prepareResponsesRequestForScheduling(c, body, "gpt-5.4")
    require.NoError(t, err)
    require.Equal(t, "gpt-5.4", patchedModel)
    require.Equal(t, service.TargetGroupExhausted, targetGroup)
    require.JSONEq(t, string(body), string(patchedBody))
}

func TestPrepareResponsesRequestForScheduling_SysModelAppendsContinuation(t *testing.T) {
    gin.SetMode(gin.TestMode)
    rec := httptest.NewRecorder()
    c, _ := gin.CreateTestContext(rec)

    body := []byte(`{"model":"gpt-5.4-Sys","input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"hello"}]}]}`)

    patchedBody, patchedModel, targetGroup, err := prepareResponsesRequestForScheduling(c, body, "gpt-5.4-Sys")
    require.NoError(t, err)
    require.Equal(t, "gpt-5.4", patchedModel)
    require.Equal(t, service.TargetGroupExhausted, targetGroup)
    require.Contains(t, string(patchedBody), `"type":"tool_call"`)
    require.Contains(t, string(patchedBody), `"output":"ready"`)
}

func TestResponsesNoAvailableAccountsError(t *testing.T) {
    status, code, message := responsesNoAvailableAccountsError(service.TargetGroupExhausted)
    require.Equal(t, http.StatusTooManyRequests, status)
    require.Equal(t, "rate_limit_exceeded", code)
    require.Equal(t, "No available accounts in target group (exhausted)", message)

    status, code, message = responsesNoAvailableAccountsError(service.TargetGroupActive)
    require.Equal(t, http.StatusServiceUnavailable, status)
    require.Equal(t, "service_unavailable", code)
    require.Equal(t, "No available accounts in target group (active)", message)
}
```

- [ ] **Step 2: 运行失败测试，确认 handler 预处理 helper 还不存在**

Run: `go test ./internal/handler -run "Test(PrepareResponsesRequestForScheduling|ResponsesNoAvailableAccountsError)" -count=1`
Expected: FAIL，报新 helper 未定义，或 `-Sys` 请求还不会被改写为 exhausted 路由。

- [ ] **Step 3: 在 `openai_gateway_handler.go` 中抽出可测试 helper，并复用已有 cache key**

```go
func getResponsesRequestBody(c *gin.Context, body []byte) (map[string]any, error) {
    if c != nil {
        if cached, ok := c.Get(service.OpenAIParsedRequestBodyKey); ok {
            if reqBody, ok := cached.(map[string]any); ok && reqBody != nil {
                return reqBody, nil
            }
        }
    }

    var reqBody map[string]any
    if err := json.Unmarshal(body, &reqBody); err != nil {
        return nil, err
    }
    if c != nil {
        c.Set(service.OpenAIParsedRequestBodyKey, reqBody)
    }
    return reqBody, nil
}

var errPrepareResponsesRequestParse = errors.New("prepare responses request parse")
var errPrepareResponsesRequestRewrite = errors.New("prepare responses request rewrite")

func prepareResponsesRequestForScheduling(c *gin.Context, body []byte, reqModel string) ([]byte, string, service.AccountTargetGroup, error) {
    targetGroup := service.TargetGroupActive
    needsParsedBody := service.IsSysModel(reqModel) || gjson.GetBytes(body, `input.#(type=="function_call_output")`).Exists()
    if !needsParsedBody {
        return body, reqModel, targetGroup, nil
    }

    reqBody, err := getResponsesRequestBody(c, body)
    if err != nil {
        return nil, "", service.TargetGroupAny, fmt.Errorf("%w: %v", errPrepareResponsesRequestParse, err)
    }
    if service.IsSysModel(reqModel) && service.NeedsSysToolContinuation(reqBody) {
        service.AppendMinimalSysToolContinuation(reqBody)
        patchedBody, err := json.Marshal(reqBody)
        if err != nil {
            return nil, "", service.TargetGroupAny, fmt.Errorf("%w: %v", errPrepareResponsesRequestRewrite, err)
        }
        body = patchedBody
        c.Set(service.OpenAIParsedRequestBodyKey, reqBody)
    }

    if service.IsSysModel(reqModel) {
        reqModel = service.StripSysSuffix(reqModel)
    }
    targetGroup = service.GetRequestTargetGroup(reqBody)
    return body, reqModel, targetGroup, nil
}

func isResponsesNoAvailableAccountsError(err error) bool {
    return err != nil && (errors.Is(err, service.ErrNoAvailableAccounts) ||
        strings.Contains(strings.ToLower(err.Error()), "no available openai accounts"))
}

func responsesNoAvailableAccountsError(targetGroup service.AccountTargetGroup) (int, string, string) {
    if targetGroup == service.TargetGroupExhausted {
        return http.StatusTooManyRequests, "rate_limit_exceeded", "No available accounts in target group (exhausted)"
    }
    return http.StatusServiceUnavailable, "service_unavailable", "No available accounts in target group (active)"
}
```

- [ ] **Step 4: 把 helper 接回 `Responses()` 主流程，并仅在无可用账号场景映射分组错误**

```go
body, reqModel, targetGroup, err := prepareResponsesRequestForScheduling(c, body, reqModel)
if err != nil {
    switch {
    case errors.Is(err, errPrepareResponsesRequestParse):
        h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "Failed to parse request body")
    default:
        h.errorResponse(c, http.StatusInternalServerError, "server_error", "Failed to rewrite request body")
    }
    return
}

selection, scheduleDecision, err := h.gatewayService.SelectAccountWithScheduler(
    c.Request.Context(),
    apiKey.GroupID,
    previousResponseID,
    sessionHash,
    reqModel,
    targetGroup,
    failedAccountIDs,
    service.OpenAIUpstreamTransportAny,
)
if err != nil && isResponsesNoAvailableAccountsError(err) {
    if len(failedAccountIDs) == 0 {
        status, code, message := responsesNoAvailableAccountsError(targetGroup)
        h.handleStreamingAwareError(c, status, code, message, streamStarted)
        return
    }
    if lastFailoverErr != nil {
        h.handleFailoverExhausted(c, lastFailoverErr, streamStarted)
    } else {
        h.handleFailoverExhaustedSimple(c, 502, streamStarted)
    }
    return
}
if err != nil {
    reqLog.Warn("openai.account_select_failed",
        zap.Error(err),
        zap.Int("excluded_account_count", len(failedAccountIDs)),
    )
    if len(failedAccountIDs) == 0 {
        h.handleStreamingAwareError(c, http.StatusServiceUnavailable, "api_error", "Service temporarily unavailable", streamStarted)
        return
    }
    if lastFailoverErr != nil {
        h.handleFailoverExhausted(c, lastFailoverErr, streamStarted)
    } else {
        h.handleFailoverExhaustedSimple(c, 502, streamStarted)
    }
    return
}
if selection == nil || selection.Account == nil {
    status, code, message := responsesNoAvailableAccountsError(targetGroup)
    h.handleStreamingAwareError(c, status, code, message, streamStarted)
    return
}
```

保持 `sessionHashBody` 继续使用原始请求体，不要因为 `-Sys` 注入而改变现有 session hash 语义。

- [ ] **Step 5: 重新运行 handler 定向测试，确认 `Responses()` 预处理语义对齐 spec**

Run: `go test ./internal/handler -run "Test(PrepareResponsesRequestForScheduling|ResponsesNoAvailableAccountsError)" -count=1`
Expected: PASS，`function_call_output` 请求进入 exhausted 组，`-Sys` 用户消息请求会先补 `tool_call + function_call_output` 再进入 exhausted 组，错误映射为 exhausted=`429` / active=`503`。

- [ ] **Step 6: 提交 `Responses()` 预处理变更**

```bash
git add internal/handler/openai_gateway_handler.go internal/handler/openai_gateway_handler_test.go
git commit -m "feat(handler): 接入 Responses 目标组路由"
```

### Task 5: 在 `/v1/models` 的两条 OpenAI 分支都暴露 `-Sys` 别名

**Files:**
- Create: `internal/handler/gateway_handler_models_test.go`
- Modify: `internal/handler/gateway_handler.go`

- [ ] **Step 1: 先写失败测试，分别锁定 `[]openai.Model` 与 `[]claude.Model` 的别名展开**

```go
func TestExpandOpenAISysAliases(t *testing.T) {
    models := []openai.Model{{ID: "gpt-5.4", Object: "model", Created: 1, OwnedBy: "openai", Type: "model", DisplayName: "GPT-5.4"}}

    expanded := expandOpenAISysAliases(models)

    require.Len(t, expanded, 2)
    require.Equal(t, "gpt-5.4", expanded[0].ID)
    require.Equal(t, "gpt-5.4-Sys", expanded[1].ID)
    require.Equal(t, "GPT-5.4 (Sys)", expanded[1].DisplayName)
}

func TestExpandClaudeSysAliases(t *testing.T) {
    models := []claude.Model{
        {ID: "gpt-5.4", Type: "model", DisplayName: "gpt-5.4", CreatedAt: "2024-01-01T00:00:00Z"},
        {ID: "GPT-5.4-sys", Type: "model", DisplayName: "gpt-5.4 (Sys)", CreatedAt: "2024-01-01T00:00:00Z"},
    }

    expanded := expandClaudeSysAliases(models)

    require.Len(t, expanded, 2)
    require.Equal(t, "gpt-5.4", expanded[0].ID)
    require.Equal(t, "GPT-5.4-sys", expanded[1].ID)
}
```

- [ ] **Step 2: 运行失败测试，确认当前 `Models()` 还没有别名 helper**

Run: `go test ./internal/handler -run "TestExpand(OpenAISysAliases|ClaudeSysAliases)" -count=1`
Expected: FAIL，helper 不存在，或没有做到大小写去重与“原模型后紧邻别名”。

- [ ] **Step 3: 在 `gateway_handler.go` 中补两套 helper，并接入 `Models()` 的 whitelist/fallback 两条 OpenAI 分支**

```go
func expandOpenAISysAliases(models []openai.Model) []openai.Model {
    expanded := make([]openai.Model, 0, len(models)*2)
    seen := make(map[string]struct{}, len(models)*2)
    for _, model := range models {
        key := strings.ToLower(model.ID)
        if _, ok := seen[key]; ok {
            continue
        }
        expanded = append(expanded, model)
        seen[key] = struct{}{}

        if service.IsSysModel(model.ID) {
            continue
        }
        alias := model
        alias.ID = model.ID + "-Sys"
        alias.DisplayName = model.DisplayName + " (Sys)"
        aliasKey := strings.ToLower(alias.ID)
        if _, ok := seen[aliasKey]; ok {
            continue
        }
        expanded = append(expanded, alias)
        seen[aliasKey] = struct{}{}
    }
    return expanded
}

func expandClaudeSysAliases(models []claude.Model) []claude.Model {
    expanded := make([]claude.Model, 0, len(models)*2)
    seen := make(map[string]struct{}, len(models)*2)
    for _, model := range models {
        key := strings.ToLower(model.ID)
        if _, ok := seen[key]; ok {
            continue
        }
        expanded = append(expanded, model)
        seen[key] = struct{}{}

        if service.IsSysModel(model.ID) {
            continue
        }
        alias := model
        alias.ID = model.ID + "-Sys"
        alias.DisplayName = model.DisplayName + " (Sys)"
        aliasKey := strings.ToLower(alias.ID)
        if _, ok := seen[aliasKey]; ok {
            continue
        }
        expanded = append(expanded, alias)
        seen[aliasKey] = struct{}{}
    }
    return expanded
}
```

```go
if len(availableModels) > 0 {
    models := make([]claude.Model, 0, len(availableModels))
    for _, modelID := range availableModels {
        models = append(models, claude.Model{
            ID:          modelID,
            Type:        "model",
            DisplayName: modelID,
            CreatedAt:   "2024-01-01T00:00:00Z",
        })
    }
    if platform == service.PlatformOpenAI {
        models = expandClaudeSysAliases(models)
    }
    c.JSON(http.StatusOK, gin.H{"object": "list", "data": models})
    return
}

if platform == service.PlatformOpenAI {
    c.JSON(http.StatusOK, gin.H{
        "object": "list",
        "data":   expandOpenAISysAliases(openai.DefaultModels),
    })
    return
}
```

- [ ] **Step 4: 重新运行模型列表测试，确认两条 OpenAI 分支都满足 spec**

Run: `go test ./internal/handler -run "TestExpand(OpenAISysAliases|ClaudeSysAliases)" -count=1`
Expected: PASS，原模型保留原顺序，`-Sys` 紧跟原模型，大小写重复别名不会再次生成。

- [ ] **Step 5: 提交模型列表变更**

```bash
git add internal/handler/gateway_handler.go internal/handler/gateway_handler_models_test.go
git commit -m "feat(models): 暴露 Sys 模型别名"
```

### Task 6: 做完整回归并确认没有漏掉任何签名变更

**Files:**
- Modify: `internal/service/account_target_group_test.go`
- Modify: `internal/service/openai_account_scheduler_test.go`
- Modify: `internal/service/openai_ws_account_sticky_test.go`
- Modify: `internal/service/openai_tool_continuation_test.go`
- Modify: `internal/service/openai_model_mapping_test.go`
- Modify: `internal/handler/openai_gateway_handler_test.go`
- Modify: `internal/handler/gateway_handler_models_test.go`

- [ ] **Step 1: 跑 service 包完整回归，确认新增 target-group 逻辑不打穿已有调度行为**

Run: `go test ./internal/service -count=1`
Expected: PASS，既有 scheduler / gateway service / previous-response / codex snapshot 测试全部通过，新增 target-group 行为没有破坏 sticky、load-balance 与 failover。

- [ ] **Step 2: 跑 handler 包完整回归，确认 `Responses()`、`Messages()`、`ResponsesWebSocket()` 与 `/v1/models` 全部通过**

Run: `go test ./internal/handler -count=1`
Expected: PASS，handler 包内旧测试和新增 helper/models 测试全部通过。

- [ ] **Step 3: 跑整仓编译与测试，确认没有漏掉 `SelectAccountWithScheduler(...)` / `SelectAccountByPreviousResponseID(...)` 新参数**

Run: `go test ./... -count=1`
Expected: PASS，没有任何调用点因为新增 `TargetGroup` 参数而漏改，也没有新的编译错误。

- [ ] **Step 4: 执行格式化并检查变更面**

Run: `gofmt -w internal/service/account.go internal/service/account_target_group_test.go internal/service/openai_account_scheduler.go internal/service/openai_gateway_service.go internal/service/openai_ws_forwarder.go internal/service/openai_account_scheduler_test.go internal/service/openai_ws_account_sticky_test.go internal/service/openai_tool_continuation.go internal/service/openai_tool_continuation_test.go internal/service/openai_model_mapping.go internal/service/openai_model_mapping_test.go internal/handler/openai_gateway_handler.go internal/handler/openai_gateway_handler_test.go internal/handler/openai_chat_completions.go internal/handler/gateway_handler.go internal/handler/gateway_handler_models_test.go`
Expected: 无输出；随后 `git diff --stat` 只包含计划中列出的文件。

- [ ] **Step 5: 提交最终集成结果**

```bash
git add internal/service/account.go internal/service/account_target_group_test.go internal/service/openai_account_scheduler.go internal/service/openai_gateway_service.go internal/service/openai_ws_forwarder.go internal/service/openai_account_scheduler_test.go internal/service/openai_ws_account_sticky_test.go internal/service/openai_tool_continuation.go internal/service/openai_tool_continuation_test.go internal/service/openai_model_mapping.go internal/service/openai_model_mapping_test.go internal/handler/openai_gateway_handler.go internal/handler/openai_gateway_handler_test.go internal/handler/openai_chat_completions.go internal/handler/gateway_handler.go internal/handler/gateway_handler_models_test.go
git commit -m "feat(openai): 完成目标组路由与 Sys 模型支持"
```

## 覆盖检查

- OAuth exhausted 判定使用 `codex_7d_used_percent >= 100` 或 `codex_primary_used_percent >= 100`，API Key 继续沿用 `IsQuotaExceeded()`，已由 Task 1 覆盖。
- exhausted 组选号时跳过 `RateLimitResetAt`，但仍保留 `expired / overload / temp unschedulable` 检查，已由 Task 1 与 Task 2 覆盖。
- sticky / previous-response 目标组不匹配时只跳过、不清缓存，真实不可调度时才清缓存，已由 Task 2 覆盖。
- load-balance、snapshot fresh resolve、DB recheck 三层都改成 `IsSchedulableForTargetGroup(...)`，避免 exhausted 账号在二次校验阶段重新被普通 `IsSchedulable()` 排除，已由 Task 2 覆盖。
- `-Sys` 识别、去后缀、`tool_call + function_call_output` 最小续链注入、`output == "ready"`，已由 Task 3 覆盖。
- `Responses()` 在 `-Sys` 注入之后再计算 `TargetGroup`，因此 `-Sys` 用户消息请求会进入 exhausted 组，已由 Task 4 覆盖。
- `/v1/models` 的 OpenAI whitelist 分支和 OpenAI fallback 分支都追加 `-Sys` 别名，已由 Task 5 覆盖。
- 非 Responses 入口（`Messages()`、`ResponsesWebSocket()`、`ChatCompletions()`）显式传 `TargetGroupAny` 保持旧行为，已由 Task 2 与 Task 6 覆盖。

## 执行注意事项

- `TargetGroupAny` 保持零值 `""`，不要引入额外的字符串状态，否则旧调用点与日志会多出无意义分支。
- 不要在 service 聚合层改 `GetAvailableModels()`；`-Sys` 别名只在 `GatewayHandler.Models()` 的响应整形层追加。
- `Responses()` 里只在必要时解析 body：`-Sys` 请求或已经包含 `function_call_output` 的请求才走 `json.Unmarshal`，其余请求直接保持 `active`。
- `sessionHashBody` 继续使用原始请求体，避免 `-Sys` 自动注入改变 sticky/session hash 行为。
- `SelectAccountByPreviousResponseID(...)` 现有 `force_http` 分支会直接返回 `nil, nil` 且不删绑定；引入目标组逻辑时保留这条现有语义。
- VPS 部署不在本计划内；先完成代码、测试和本地验证，再进入部署流程。
