package service

import (
	"context"
	"errors"
	"math"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type expiryResetTestQuota struct {
	autoResetTestQuota
	result    *OpenAIQuotaResetResult
	queryHook func()
	postErr   error
}

func (q *expiryResetTestQuota) QueryUsage(ctx context.Context, id int64) (*OpenAIQuotaUsage, error) {
	usage, err := q.autoResetTestQuota.QueryUsage(ctx, id)
	if q.queryHook != nil {
		q.queryHook()
	}
	return usage, err
}

func (q *expiryResetTestQuota) ResetCreditTargeted(ctx context.Context, id int64, creditID, redeemID string) (*OpenAIQuotaResetResult, error) {
	result, err := q.autoResetTestQuota.ResetCreditTargeted(ctx, id, creditID, redeemID)
	if err == nil && q.result != nil {
		return q.result, nil
	}
	return result, err
}

func (q *expiryResetTestQuota) CachePostResetSnapshot(context.Context, int64, *OpenAIQuotaUsage) error {
	return q.postErr
}

type expiryResetTestRecoverer struct{ calls atomic.Int32 }

func (r *expiryResetTestRecoverer) RecoverAccountState(context.Context, int64, AccountRecoveryOptions) (*SuccessfulTestRecoveryResult, error) {
	r.calls.Add(1)
	return &SuccessfulTestRecoveryResult{}, nil
}

func newExpiryResetFixture(t *testing.T) (*OpenAIQuotaAutoResetService, *autoResetTestAccountRepo, *expiryResetTestQuota, *expiryResetTestRecoverer) {
	t.Helper()
	now := time.Now().UTC()
	account := &Account{ID: 601, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true,
		Extra: map[string]any{OpenAIAutoResetCreditExpiryEnabledExtraKey: true, OpenAIAutoResetCreditExpiryHoursExtraKey: 1.0,
			"codex_usage_updated_at": now.Format(time.RFC3339), "codex_5h_used_percent": 2.0}}
	repo := &autoResetTestAccountRepo{account: account}
	quota := &expiryResetTestQuota{autoResetTestQuota: autoResetTestQuota{usage: &OpenAIQuotaUsage{
		FetchedAt: now.Unix(),
		RateLimit: &OpenAIRateLimit{PrimaryWindow: &OpenAIRateLimitWindow{
			UsedPercent: 2, LimitWindowSeconds: 5 * 3600, ResetAt: now.Add(2 * time.Hour).Unix(), ResetAfterSeconds: 7200,
		}},
		RateLimitResetCredits: &OpenAIRateLimitResetCredits{AvailableCount: 1, Credits: []OpenAIRateLimitResetCreditDetail{{ExpiresAt: now.Add(30 * time.Minute).Format(time.RFC3339)}}},
		autoResetCandidates:   []openAIAutoResetCreditCandidate{{ID: "expiry-card", ExpiresAt: now.Add(30 * time.Minute).Format(time.RFC3339)}},
	}}}
	recoverer := &expiryResetTestRecoverer{}
	cfg := DefaultIdempotencyConfig()
	cfg.ObserveOnly = false
	cfg.FailedRetryBackoff = 0
	svc := NewOpenAIQuotaAutoResetService(repo, quota, recoverer, NewIdempotencyCoordinator(newInMemoryIdempotencyRepo(), cfg), nil, nil, nil)
	return svc, repo, quota, recoverer
}

func TestNormalizeOpenAIAutoResetExpiryConfig(t *testing.T) {
	config := ResolveOpenAIAutoResetCreditConfig(&Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth})
	require.False(t, config.ExpiryEnabled)
	require.Equal(t, 1.0, config.ExpiryHours)
	for _, hours := range []float64{0.5, 1, 200, 100000} {
		extra, err := normalizeOpenAIAutoResetCreditExtra(PlatformOpenAI, AccountTypeOAuth, false, map[string]any{
			OpenAIAutoResetCreditExpiryEnabledExtraKey: true, OpenAIAutoResetCreditExpiryHoursExtraKey: hours,
		})
		require.NoError(t, err)
		require.Equal(t, hours, extra[OpenAIAutoResetCreditExpiryHoursExtraKey])
	}
	for _, hours := range []float64{0, -1, math.NaN(), math.Inf(1), math.MaxFloat64, 1e-20} {
		_, err := normalizeOpenAIAutoResetCreditExtra(PlatformOpenAI, AccountTypeOAuth, false, map[string]any{OpenAIAutoResetCreditExpiryHoursExtraKey: hours})
		require.Error(t, err)
	}
	extra, err := normalizeOpenAIAutoResetCreditExtra(PlatformOpenAI, AccountTypeOAuth, false, map[string]any{OpenAIAutoResetCreditExpiryEnabledExtraKey: true})
	require.NoError(t, err)
	require.Equal(t, 1.0, extra[OpenAIAutoResetCreditExpiryHoursExtraKey])
	for _, bad := range []map[string]any{
		{OpenAIAutoResetCreditExpiryEnabledExtraKey: "true"},
		{OpenAIAutoResetCreditExpiryHoursExtraKey: "NaN"},
	} {
		_, err = normalizeOpenAIAutoResetCreditExtra(PlatformOpenAI, AccountTypeOAuth, false, bad)
		require.Error(t, err)
	}
	_, err = normalizeOpenAIAutoResetCreditExtra(PlatformOpenAI, AccountTypeAPIKey, false, extra)
	require.Error(t, err)
	stripped := stripOpenAIAutoResetCreditManagedExtra(extra, true)
	require.NotContains(t, stripped, OpenAIAutoResetCreditExpiryEnabledExtraKey)
	require.NotContains(t, stripped, OpenAIAutoResetCreditExpiryHoursExtraKey)
}

func TestOpenAIAutoResetExpiryWindowBoundary(t *testing.T) {
	now := time.Date(2026, 10, 29, 0, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		offset time.Duration
		want   bool
	}{
		{-time.Second, false}, {0, false}, {time.Second, true}, {30 * time.Minute, true}, {30*time.Minute + time.Second, false},
	} {
		require.Equal(t, test.want, openAIAutoResetCreditExpiring(now.Add(test.offset).Format(time.RFC3339), 0.5, now))
	}
	require.False(t, openAIAutoResetCreditExpiring("invalid", 1, now))
}

func TestOpenAIQuotaAutoResetService_ExpiryIgnoresSchedulingState(t *testing.T) {
	for _, status := range []string{StatusActive, StatusDisabled, StatusError} {
		t.Run(status, func(t *testing.T) {
			svc, repo, quota, _ := newExpiryResetFixture(t)
			past := time.Now().Add(-24 * time.Hour)
			repo.account.Status = status
			repo.account.Schedulable = false
			repo.account.ExpiresAt = &past
			repo.account.AutoPauseOnExpired = true
			require.NoError(t, svc.evaluateAccount(context.Background(), 601))
			require.Equal(t, int32(1), quota.resetCalls.Load())
			require.Equal(t, openAIAutoResetTriggerExpiry, repo.stateForTest().TriggerReason)
		})
	}
	for _, change := range []func(*Account){
		func(a *Account) { a.Extra[OpenAIAutoResetCreditExpiryEnabledExtraKey] = false },
		func(a *Account) { a.Type = AccountTypeAPIKey },
		func(a *Account) { id := int64(1); a.ParentAccountID = &id },
	} {
		svc, repo, quota, _ := newExpiryResetFixture(t)
		change(repo.account)
		require.NoError(t, svc.evaluateAccount(context.Background(), 601))
		require.Zero(t, quota.resetCalls.Load())
	}
}

func TestOpenAIQuotaAutoResetService_ExpiryRechecksConfigAfterQuery(t *testing.T) {
	for _, change := range []func(*Account){
		func(a *Account) { a.Extra[OpenAIAutoResetCreditExpiryEnabledExtraKey] = false },
		func(a *Account) { a.Extra[OpenAIAutoResetCreditExpiryHoursExtraKey] = 0.1 },
		func(a *Account) { a.Type = AccountTypeAPIKey },
	} {
		svc, repo, quota, _ := newExpiryResetFixture(t)
		quota.queryHook = func() { repo.mu.Lock(); defer repo.mu.Unlock(); change(repo.account) }
		require.NoError(t, svc.evaluateAccount(context.Background(), 601))
		require.Zero(t, quota.resetCalls.Load())
	}
}

func TestOpenAIQuotaAutoResetService_ExpiryQueryClockIndependentOfUsage(t *testing.T) {
	svc, repo, quota, _ := newExpiryResetFixture(t)
	now := time.Now().UTC()
	quota.usage.autoResetCandidates[0].ExpiresAt = now.Add(2 * time.Hour).Format(time.RFC3339)
	require.NoError(t, svc.evaluateAccount(context.Background(), 601))
	require.Equal(t, int32(1), quota.queryCalls.Load())
	require.Zero(t, quota.resetCalls.Load())
	for i := 0; i < 3; i++ {
		repo.setExtraForTest("codex_usage_updated_at", time.Now().UTC().Format(time.RFC3339))
		require.NoError(t, svc.evaluateAccount(context.Background(), 601))
	}
	require.Equal(t, int32(1), quota.queryCalls.Load(), "远期卡不会随每次扫描或用量刷新查询")
	state := repo.stateForTest()
	state.CreditsCheckedAt = now.Add(-2 * time.Minute).Format(time.RFC3339)
	state.NextCreditExpiresAt = now.Add(30 * time.Minute).Format(time.RFC3339)
	repo.setExtraForTest(OpenAIAutoResetCreditStateExtraKey, state)
	quota.usage.autoResetCandidates[0].ExpiresAt = state.NextCreditExpiresAt
	require.NoError(t, svc.evaluateAccount(context.Background(), 601))
	require.Equal(t, int32(1), quota.resetCalls.Load(), "进入临期窗口强制实时确认，不等用量缓存变旧")
}

func TestOpenAIQuotaAutoResetService_NoOpThenThresholdUsesDeterministicGeneration(t *testing.T) {
	svc, repo, quota, recoverer := newExpiryResetFixture(t)
	quota.result = &OpenAIQuotaResetResult{Code: "nothing_to_reset", WindowsReset: 0}
	require.NoError(t, svc.evaluateAccount(context.Background(), 601))
	require.Equal(t, OpenAIAutoResetStatusNotConsumed, repo.stateForTest().Status)
	require.Zero(t, recoverer.calls.Load(), "没有消费的结果不能解除本地调度状态")
	for i := 0; i < 3; i++ {
		require.NoError(t, svc.evaluateAccount(context.Background(), 601))
	}
	require.Equal(t, int32(1), quota.resetCalls.Load(), "到期no-op不能每分钟发出新兑换")
	repo.account.Extra[OpenAIAutoResetCreditEnabledExtraKey] = true
	repo.account.Extra["codex_5h_used_percent"] = 100.0
	quota.usage.RateLimit.PrimaryWindow.UsedPercent = 100
	quota.result = &OpenAIQuotaResetResult{Code: "ok", WindowsReset: 2}
	otherInstance := NewOpenAIQuotaAutoResetService(repo, quota, recoverer, svc.idempotency, nil, nil, nil)
	require.NoError(t, otherInstance.evaluateAccount(context.Background(), 601))
	require.Equal(t, int32(2), quota.resetCalls.Load())
	require.NotEqual(t, quota.resetArgs[0][1], quota.resetArgs[1][1])
	state := repo.stateForTest()
	require.Equal(t, openAIAutoResetAttemptConsumed, state.Attempts[0].Outcome)
	require.Equal(t, "usage:"+shortOpenAIAutoResetHash(openAIAutoResetCycleSeed(quota.usage)), state.Attempts[0].Generation)
	quota.usage.RateLimit.PrimaryWindow.ResetAt += 3600
	require.NoError(t, svc.evaluateAccount(context.Background(), 601))
	require.Equal(t, int32(2), quota.resetCalls.Load(), "已经消费的卡不能因额度周期变化而换代")
}

func TestOpenAIQuotaAutoResetService_NoOpCannotBypassUsageEligibility(t *testing.T) {
	for _, disable := range []func(*Account){
		func(a *Account) { a.Extra[OpenAIAutoResetCreditEnabledExtraKey] = false },
		func(a *Account) { a.Schedulable = false },
		func(a *Account) { a.Status = StatusDisabled },
	} {
		svc, repo, quota, _ := newExpiryResetFixture(t)
		quota.result = &OpenAIQuotaResetResult{Code: "nothing_to_reset"}
		require.NoError(t, svc.evaluateAccount(context.Background(), 601))
		repo.account.Extra[OpenAIAutoResetCreditEnabledExtraKey] = true
		repo.account.Extra["codex_5h_used_percent"] = 100.0
		quota.usage.RateLimit.PrimaryWindow.UsedPercent = 100
		disable(repo.account)
		state := repo.stateForTest()
		state.CreditsCheckedAt = time.Now().Add(-11 * time.Minute).UTC().Format(time.RFC3339)
		repo.setExtraForTest(OpenAIAutoResetCreditStateExtraKey, state)
		require.NoError(t, svc.evaluateAccount(context.Background(), 601))
		require.Equal(t, int32(1), quota.resetCalls.Load())
	}
}

func TestOpenAIQuotaAutoResetService_NoCreditPreservesUnavailableState(t *testing.T) {
	svc, repo, quota, recoverer := newExpiryResetFixture(t)
	quota.result = &OpenAIQuotaResetResult{Code: "no_credit"}
	require.NoError(t, svc.evaluateAccount(context.Background(), 601))
	state := repo.stateForTest()
	require.Equal(t, OpenAIAutoResetStatusNoCredit, state.Status)
	require.Zero(t, state.AvailableCount)
	require.Equal(t, "NO_RESET_CREDIT", state.ErrorCode)
	require.Equal(t, openAIAutoResetAttemptNotConsumed, state.Attempts[0].Outcome)
	require.Zero(t, recoverer.calls.Load())
}

func TestOpenAIQuotaAutoResetService_NoCreditRemainsUnavailableAfterRefresh(t *testing.T) {
	for _, usagePath := range []bool{false, true} {
		name := "expiry"
		if usagePath {
			name = "usage threshold"
		}
		t.Run(name, func(t *testing.T) {
			svc, repo, quota, _ := newExpiryResetFixture(t)
			if usagePath {
				repo.account.Extra[OpenAIAutoResetCreditExpiryEnabledExtraKey] = false
				repo.account.Extra[OpenAIAutoResetCreditEnabledExtraKey] = true
				repo.account.Extra["codex_5h_used_percent"] = 100.0
				quota.usage.RateLimit.PrimaryWindow.UsedPercent = 100
			}
			quota.result = &OpenAIQuotaResetResult{Code: "no_credit"}
			require.NoError(t, svc.evaluateAccount(context.Background(), 601))
			require.Equal(t, OpenAIAutoResetStatusNoCredit, repo.stateForTest().Status)
			state := repo.stateForTest()
			stale := time.Now().Add(-11 * time.Minute).UTC().Format(time.RFC3339)
			state.CreditsCheckedAt, state.CheckedAt = stale, stale
			repo.setExtraForTest(OpenAIAutoResetCreditStateExtraKey, state)
			repo.setExtraForTest("codex_usage_updated_at", stale)
			quota.usage.RateLimitResetCredits.AvailableCount = 0
			quota.usage.RateLimitResetCredits.Credits = nil
			quota.usage.autoResetCandidates = nil
			require.NoError(t, svc.evaluateAccount(context.Background(), 601))
			refreshed := repo.stateForTest()
			require.Equal(t, OpenAIAutoResetStatusNoCredit, refreshed.Status)
			require.Zero(t, refreshed.AvailableCount)
			require.Equal(t, "NO_RESET_CREDIT", refreshed.ErrorCode)
			require.Equal(t, int32(2), quota.queryCalls.Load())
			require.Equal(t, int32(1), quota.resetCalls.Load())
		})
	}
}

func TestOpenAIQuotaAutoResetService_ThresholdDropClearsDynamicTrigger(t *testing.T) {
	svc, repo, quota, _ := newExpiryResetFixture(t)
	repo.account.Extra[OpenAIAutoResetCreditExpiryEnabledExtraKey] = false
	repo.account.Extra[OpenAIAutoResetCreditEnabledExtraKey] = true
	repo.setExtraForTest(OpenAIAutoResetCreditStateExtraKey, &OpenAIAutoResetCreditState{
		Status: OpenAIAutoResetStatusFailed, TriggerWindow: "5h", AvailableCount: 1, ErrorCode: "OLD_FAILURE",
	})
	require.NoError(t, svc.evaluateAccount(context.Background(), 601))
	state := repo.stateForTest()
	require.Empty(t, state.TriggerWindow)
	require.Empty(t, state.ErrorCode)
	require.Equal(t, OpenAIAutoResetStatusAvailable, state.Status)
	require.Zero(t, quota.queryCalls.Load())
}

type expiryDeletedAccountRepo struct {
	*autoResetTestAccountRepo
	deleted bool
}

func (r *expiryDeletedAccountRepo) GetByID(ctx context.Context, id int64) (*Account, error) {
	if r.deleted {
		return nil, errors.New("account not found")
	}
	return r.autoResetTestAccountRepo.GetByID(ctx, id)
}

func TestOpenAIQuotaAutoResetService_DeletedAccountCannotConsume(t *testing.T) {
	svc, repo, quota, _ := newExpiryResetFixture(t)
	deletedRepo := &expiryDeletedAccountRepo{autoResetTestAccountRepo: repo}
	svc.accountRepo = deletedRepo
	quota.queryHook = func() { deletedRepo.deleted = true }
	require.Error(t, svc.evaluateAccount(context.Background(), 601))
	require.Zero(t, quota.resetCalls.Load())
}

func TestOpenAIQuotaAutoResetService_UnknownDoesNotSwitchCardsOrIdentity(t *testing.T) {
	svc, repo, quota, _ := newExpiryResetFixture(t)
	quota.failFirst = true
	require.Error(t, svc.evaluateAccount(context.Background(), 601))
	quota.usage.RateLimit.PrimaryWindow.ResetAt += 3600
	quota.result = &OpenAIQuotaResetResult{Code: "ok", WindowsReset: 0}
	require.Error(t, svc.evaluateAccount(context.Background(), 601))
	require.Equal(t, quota.resetArgs[0], quota.resetArgs[1], "超时或未知结果保持原卡原兑换ID")
	quota.usage.autoResetCandidates = []openAIAutoResetCreditCandidate{{ID: "other-card", ExpiresAt: time.Now().Add(40 * time.Minute).UTC().Format(time.RFC3339)}}
	require.Error(t, svc.evaluateAccount(context.Background(), 601))
	require.Equal(t, int32(2), quota.resetCalls.Load(), "原卡缺失且结果不明时拒绝改用下一张")
	require.Equal(t, openAIAutoResetAttemptPending, repo.stateForTest().Attempts[0].Outcome)
}

func TestOpenAIQuotaAutoResetService_UnknownResponseRetriesSameIdentityWithOneOwner(t *testing.T) {
	svc, repo, quota, recoverer := newExpiryResetFixture(t)
	quota.usage.RateLimitResetCredits.AvailableCount = 2
	quota.usage.autoResetCandidates = append(quota.usage.autoResetCandidates,
		openAIAutoResetCreditCandidate{ID: "other-expiring-card", ExpiresAt: time.Now().Add(40 * time.Minute).UTC().Format(time.RFC3339)})
	quota.result = &OpenAIQuotaResetResult{Code: "not_available", WindowsReset: 0}
	require.Error(t, svc.evaluateAccount(context.Background(), 601))
	state := repo.stateForTest()
	require.Equal(t, openAIAutoResetAttemptPending, state.Attempts[0].Outcome)
	stableKey := openAIAutoResetAttemptKey(601, state.Attempts[0])
	record, err := svc.idempotency.repo.GetByScopeAndKeyHash(context.Background(), "openai_auto_reset_credit", HashIdempotencyKey(stableKey))
	require.NoError(t, err)
	require.Equal(t, IdempotencyStatusFailedRetryable, record.Status, "未知200不能保存成永远回放的成功")
	require.Zero(t, recoverer.calls.Load())

	quota.result = &OpenAIQuotaResetResult{Code: "ok", WindowsReset: 1}
	quota.resetEntered = make(chan struct{})
	quota.releaseReset = make(chan struct{})
	done := make(chan error, 1)
	go func() { done <- svc.evaluateAccount(context.Background(), 601) }()
	<-quota.resetEntered
	other := NewOpenAIQuotaAutoResetService(repo, quota, recoverer, svc.idempotency, nil, nil, nil)
	require.NoError(t, other.evaluateAccount(context.Background(), 601))
	close(quota.releaseReset)
	require.NoError(t, <-done)
	require.Equal(t, int32(2), quota.resetCalls.Load(), "并发确认只允许一个实例按原参数重试")
	require.Equal(t, quota.resetArgs[0], quota.resetArgs[1])
	require.Equal(t, "expiry-card", quota.resetArgs[1][0])
	require.Equal(t, int32(1), recoverer.calls.Load())
	require.Equal(t, openAIAutoResetAttemptConsumed, repo.stateForTest().Attempts[0].Outcome)
}

func TestOpenAIQuotaAutoResetService_LegacySucceededUnknownIsNotBlindlyRewritten(t *testing.T) {
	svc, repo, quota, _ := newExpiryResetFixture(t)
	attempt := OpenAIAutoResetCreditAttempt{CreditHash: shortOpenAIAutoResetHash("expiry-card"), Outcome: openAIAutoResetAttemptPending,
		CreditExpiresAt: quota.usage.autoResetCandidates[0].ExpiresAt, TriggerReason: openAIAutoResetTriggerExpiry}
	stableKey := openAIAutoResetAttemptKey(601, attempt)
	// 模拟旧候选已把未知200保存为成功；没有响应CAS时不能据此改写整个记录。
	_, err := svc.idempotency.Execute(context.Background(), IdempotencyExecuteOptions{
		Scope: "openai_auto_reset_credit", ActorScope: "account:601", Method: "POST", Route: "/system/openai/reset-credit/auto", IdempotencyKey: stableKey,
		Payload: map[string]any{"account_id": int64(601), "credit_hash": attempt.CreditHash, "generation": ""},
	}, func(context.Context) (any, error) {
		return openAIAutoResetConsumeResult{Code: "not_available", WindowsReset: 0}, nil
	})
	require.NoError(t, err)
	repo.setExtraForTest(OpenAIAutoResetCreditStateExtraKey, &OpenAIAutoResetCreditState{
		Status: OpenAIAutoResetStatusFailed, AttemptCreditHash: attempt.CreditHash, AttemptKeyVersion: openAIAutoResetCardKeyVersion,
		Attempts: []OpenAIAutoResetCreditAttempt{attempt},
	})
	require.Error(t, svc.evaluateAccount(context.Background(), 601))
	require.Zero(t, quota.resetCalls.Load())
	require.Equal(t, "RESET_RESULT_UNCONFIRMED", repo.stateForTest().ErrorCode)
	record, err := svc.idempotency.repo.GetByScopeAndKeyHash(context.Background(), "openai_auto_reset_credit", HashIdempotencyKey(stableKey))
	require.NoError(t, err)
	require.Equal(t, IdempotencyStatusSucceeded, record.Status)
	require.Equal(t, openAIAutoResetAttemptPending, repo.stateForTest().Attempts[0].Outcome)
}

func TestOpenAIQuotaAutoResetService_RefreshFailureNeverConsumesAgain(t *testing.T) {
	svc, _, quota, recoverer := newExpiryResetFixture(t)
	quota.postErr = errors.New("cache write failed")
	require.Error(t, svc.evaluateAccount(context.Background(), 601))
	quota.postErr = nil
	quota.usage.autoResetCandidates = nil
	quota.usage.RateLimitResetCredits.AvailableCount = 0
	require.NoError(t, svc.evaluateAccount(context.Background(), 601))
	require.Equal(t, int32(1), quota.resetCalls.Load())
	require.Equal(t, int32(2), recoverer.calls.Load(), "后处理失败只重试本地恢复")
}

func TestOpenAIQuotaAutoResetService_LegacyPendingRetainsExactKey(t *testing.T) {
	svc, repo, quota, _ := newExpiryResetFixture(t)
	legacyCycle := "prior-quota-cycle"
	hash := shortOpenAIAutoResetHash("expiry-card")
	repo.setExtraForTest(OpenAIAutoResetCreditStateExtraKey, &OpenAIAutoResetCreditState{
		Status: OpenAIAutoResetStatusFailed, AttemptCreditHash: hash, AttemptCycleHash: legacyCycle,
	})
	require.NoError(t, svc.evaluateAccount(context.Background(), 601))
	expected := uuid.NewSHA1(uuid.NameSpaceURL, []byte("oarc:601:"+hash+":"+legacyCycle)).String()
	require.Equal(t, expected, quota.resetArgs[0][1])
}

func TestOpenAIQuotaAutoResetService_TwoTriggersShareCardIdentity(t *testing.T) {
	svc, repo, quota, recoverer := newExpiryResetFixture(t)
	quota.resetEntered = make(chan struct{})
	quota.releaseReset = make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); _ = svc.evaluateAccount(context.Background(), 601) }()
	<-quota.resetEntered
	// 首个实例已按到期开始消费，第二实例同时收到真实用量阈值信号。
	repo.mu.Lock()
	repo.account.Extra[OpenAIAutoResetCreditEnabledExtraKey] = true
	repo.account.Extra["codex_5h_used_percent"] = 100.0
	repo.mu.Unlock()
	quota.usage.RateLimit.PrimaryWindow.UsedPercent = 100
	other := NewOpenAIQuotaAutoResetService(repo, quota, recoverer, svc.idempotency, nil, nil, nil)
	require.NoError(t, other.evaluateAccount(context.Background(), 601))
	close(quota.releaseReset)
	wg.Wait()
	require.Equal(t, int32(1), quota.resetCalls.Load())
	require.Equal(t, openAIAutoResetAttemptConsumed, repo.stateForTest().Attempts[0].Outcome)
}

func TestOpenAIQuotaAutoResetService_NoOpUsageGenerationConcurrentInstances(t *testing.T) {
	svc, repo, quota, recoverer := newExpiryResetFixture(t)
	quota.result = &OpenAIQuotaResetResult{Code: "nothing_to_reset"}
	require.NoError(t, svc.evaluateAccount(context.Background(), 601))
	repo.account.Extra[OpenAIAutoResetCreditEnabledExtraKey] = true
	repo.account.Extra["codex_5h_used_percent"] = 100.0
	quota.usage.RateLimit.PrimaryWindow.UsedPercent = 100
	quota.result = &OpenAIQuotaResetResult{Code: "nothing_to_reset"}
	quota.resetEntered = make(chan struct{})
	quota.releaseReset = make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); _ = svc.evaluateAccount(context.Background(), 601) }()
	<-quota.resetEntered
	other := NewOpenAIQuotaAutoResetService(repo, quota, recoverer, svc.idempotency, nil, nil, nil)
	require.NoError(t, other.evaluateAccount(context.Background(), 601))
	close(quota.releaseReset)
	wg.Wait()
	require.Equal(t, int32(2), quota.resetCalls.Load(), "两个实例对同一usage代次只发送一次消费")
	require.NoError(t, other.evaluateAccount(context.Background(), 601))
	require.Equal(t, int32(2), quota.resetCalls.Load(), "同一周期的nothing_to_reset不能继续换代")
}

func TestOpenAIQuotaAutoResetService_MultipleExpiryCreditsOnePerScan(t *testing.T) {
	svc, repo, quota, _ := newExpiryResetFixture(t)
	quota.result = &OpenAIQuotaResetResult{Code: "nothing_to_reset"}
	quota.usage.RateLimitResetCredits.AvailableCount = 2
	quota.usage.autoResetCandidates = append(quota.usage.autoResetCandidates,
		openAIAutoResetCreditCandidate{ID: "later-card", ExpiresAt: time.Now().Add(40 * time.Minute).UTC().Format(time.RFC3339)})
	require.NoError(t, svc.evaluateAccount(context.Background(), 601))
	require.Equal(t, int32(1), quota.resetCalls.Load())
	require.Equal(t, "expiry-card", quota.resetArgs[0][0])
	state := repo.stateForTest()
	state.CreditsCheckedAt = time.Now().Add(-2 * time.Minute).UTC().Format(time.RFC3339)
	repo.setExtraForTest(OpenAIAutoResetCreditStateExtraKey, state)
	require.NoError(t, svc.evaluateAccount(context.Background(), 601))
	require.Equal(t, int32(2), quota.resetCalls.Load())
	require.Equal(t, "later-card", quota.resetArgs[1][0], "前卡no-op不得堵塞其他临期卡")
}

func TestOpenAIQuotaAutoResetService_RestartResolvesStoredSuccessWithoutOriginalCard(t *testing.T) {
	svc, repo, quota, recoverer := newExpiryResetFixture(t)
	require.NoError(t, svc.evaluateAccount(context.Background(), 601))
	state := repo.stateForTest()
	state.Status = OpenAIAutoResetStatusResetting
	state.Attempts[0].Outcome = openAIAutoResetAttemptPending
	repo.setExtraForTest(OpenAIAutoResetCreditStateExtraKey, state)
	quota.usage.autoResetCandidates = []openAIAutoResetCreditCandidate{{ID: "next-card", ExpiresAt: time.Now().Add(40 * time.Minute).UTC().Format(time.RFC3339)}}
	restarted := NewOpenAIQuotaAutoResetService(repo, quota, recoverer, svc.idempotency, nil, nil, nil)
	require.NoError(t, restarted.evaluateAccount(context.Background(), 601))
	require.Equal(t, int32(1), quota.resetCalls.Load(), "重启后先恢复已知结果，本轮不消费其他卡")
	require.Equal(t, openAIAutoResetAttemptConsumed, repo.stateForTest().Attempts[0].Outcome)
}

func TestOpenAIQuotaAutoResetService_RestartFinishesConsumedBeforePostProcess(t *testing.T) {
	svc, repo, quota, recoverer := newExpiryResetFixture(t)
	require.NoError(t, svc.evaluateAccount(context.Background(), 601))
	state := repo.stateForTest()
	// 模拟已消费事实落盘，但在首次后处理前退出；尚无 warning/error 可用于推断。
	state.PostProcessPending = true
	state.ErrorCode = ""
	state.Attempts[0].CreditExpiresAt = time.Now().Add(-time.Minute).UTC().Format(time.RFC3339)
	repo.setExtraForTest(OpenAIAutoResetCreditStateExtraKey, state)
	quota.usage.autoResetCandidates = nil
	quota.usage.RateLimitResetCredits.AvailableCount = 0
	quota.resetCalls.Store(0)
	recoverer.calls.Store(0)
	restarted := NewOpenAIQuotaAutoResetService(repo, quota, recoverer, svc.idempotency, nil, nil, nil)
	require.NoError(t, restarted.evaluateAccount(context.Background(), 601))
	require.Zero(t, quota.resetCalls.Load(), "只回放原DB消费结果，不再次请求上游消费")
	require.Equal(t, int32(1), recoverer.calls.Load())
	require.False(t, repo.stateForTest().PostProcessPending)
	require.Equal(t, openAIAutoResetAttemptConsumed, repo.stateForTest().Attempts[0].Outcome)
}

func TestOpenAIQuotaAutoResetService_PostProcessRecoveryPrecedesFailedQuotaQuery(t *testing.T) {
	svc, repo, quota, recoverer := newExpiryResetFixture(t)
	require.NoError(t, svc.evaluateAccount(context.Background(), 601))
	state := repo.stateForTest()
	state.PostProcessPending = true
	state.ErrorCode = ""
	state.AvailableCount = 1
	repo.setExtraForTest(OpenAIAutoResetCreditStateExtraKey, state)
	quota.queryErr = errors.New("upstream quota temporarily unavailable")
	quota.resetCalls.Store(0)
	quota.queryCalls.Store(0)
	recoverer.calls.Store(0)
	restarted := NewOpenAIQuotaAutoResetService(repo, quota, recoverer, svc.idempotency, nil, nil, nil)
	require.Error(t, restarted.evaluateAccount(context.Background(), 601))
	require.Equal(t, int32(1), recoverer.calls.Load(), "上游额度查询失败前已执行本地解除限流")
	require.Equal(t, int32(1), quota.queryCalls.Load(), "无需先查询额度再进入已确认消费的恢复")
	require.Zero(t, quota.resetCalls.Load())
	failed := repo.stateForTest()
	require.True(t, failed.PostProcessPending)
	require.Equal(t, OpenAIQuotaResetWarningCacheRefreshFailed, failed.ErrorCode)
	require.Equal(t, 1, failed.AvailableCount, "查询失败时保留已有数量，不伪造新快照")

	quota.queryErr = nil
	quota.usage.RateLimitResetCredits.AvailableCount = 0
	quota.usage.autoResetCandidates = nil
	require.NoError(t, restarted.evaluateAccount(context.Background(), 601))
	require.Equal(t, int32(2), recoverer.calls.Load())
	require.Zero(t, quota.resetCalls.Load())
	require.False(t, repo.stateForTest().PostProcessPending)
	require.Zero(t, repo.stateForTest().AvailableCount)
}

type expiryScanTestRepo struct {
	*autoResetTestAccountRepo
	statusFilter string
}

func (r *expiryScanTestRepo) ListWithFilters(_ context.Context, _ pagination.PaginationParams, _, _, status, _ string, _ int64, _ string) ([]Account, *pagination.PaginationResult, error) {
	r.statusFilter = status
	return []Account{*r.account}, nil, nil
}

func TestOpenAIQuotaAutoResetService_ScannerIncludesUnscheduledDisabledExpiry(t *testing.T) {
	svc, repo, _, _ := newExpiryResetFixture(t)
	repo.account.Status = StatusDisabled
	repo.account.Schedulable = false
	scanRepo := &expiryScanTestRepo{autoResetTestAccountRepo: repo}
	svc.accountRepo = scanRepo
	svc.scanEnabledAccounts(context.Background())
	require.Empty(t, scanRepo.statusFilter)
	require.Equal(t, int64(601), <-svc.queue)
}
