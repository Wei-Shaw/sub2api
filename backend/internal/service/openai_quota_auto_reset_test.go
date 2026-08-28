package service

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestNormalizeOpenAIAutoResetCreditExtra(t *testing.T) {
	t.Run("历史账号默认关闭", func(t *testing.T) {
		account := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}
		config := ResolveOpenAIAutoResetCreditConfig(account)
		require.False(t, config.Enabled)
		require.Equal(t, 1.0, config.Threshold5h)
		require.Equal(t, 1.0, config.Threshold7d)
	})

	t.Run("开启时补齐两个百分百阈值并剥离运行态", func(t *testing.T) {
		extra, err := normalizeOpenAIAutoResetCreditExtra(PlatformOpenAI, AccountTypeOAuth, false, map[string]any{
			OpenAIAutoResetCreditEnabledExtraKey: true,
			OpenAIAutoResetCreditStateExtraKey:   map[string]any{"status": "success"},
			OpenAIAutoResetCreditExpiryTargetExtraKey: map[string]any{
				"credit_id": "forged",
			},
		})
		require.NoError(t, err)
		require.Equal(t, 1.0, extra[OpenAIAutoResetCredit5hThresholdExtraKey])
		require.Equal(t, 1.0, extra[OpenAIAutoResetCredit7dThresholdExtraKey])
		require.NotContains(t, extra, OpenAIAutoResetCreditStateExtraKey)
		require.NotContains(t, extra, OpenAIAutoResetCreditExpiryTargetExtraKey)
	})

	t.Run("阈值和账号类型严格校验", func(t *testing.T) {
		_, err := normalizeOpenAIAutoResetCreditExtra(PlatformOpenAI, AccountTypeOAuth, false, map[string]any{
			OpenAIAutoResetCreditEnabledExtraKey:     true,
			OpenAIAutoResetCredit5hThresholdExtraKey: 0.0009,
		})
		require.Error(t, err)

		_, err = normalizeOpenAIAutoResetCreditExtra(PlatformOpenAI, AccountTypeOAuth, true, map[string]any{
			OpenAIAutoResetCreditEnabledExtraKey: true,
		})
		require.Error(t, err)
	})
}

func TestShouldAutoPauseOpenAIAccountByQuota_AutoResetCreditStates(t *testing.T) {
	now := time.Now().UTC()
	baseExtra := map[string]any{
		OpenAIAutoResetCreditEnabledExtraKey:     true,
		OpenAIAutoResetCredit5hThresholdExtraKey: 1.0,
		OpenAIAutoResetCredit7dThresholdExtraKey: 1.0,
		"auto_pause_5h_threshold":                0.8,
		"auto_pause_7d_disabled":                 true,
		"codex_5h_used_percent":                  90.0,
		"codex_usage_updated_at":                 now.Format(time.RFC3339),
		"codex_5h_reset_at":                      now.Add(time.Hour).Format(time.RFC3339),
	}

	t.Run("卡状态未知时暂停并触发异步查询", func(t *testing.T) {
		account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: cloneOpenAIAutoResetExtra(baseExtra)}
		paused, decision := shouldAutoPauseOpenAIAccountByQuota(context.Background(), account)
		require.True(t, paused)
		require.Equal(t, "quota_auto_reset_credit_check_5h", decision.reason)
	})

	t.Run("明确有卡时允许继续到用卡阈值", func(t *testing.T) {
		extra := cloneOpenAIAutoResetExtra(baseExtra)
		extra[OpenAIAutoResetCreditStateExtraKey] = OpenAIAutoResetCreditState{
			Status: OpenAIAutoResetStatusAvailable, AvailableCount: 1, CheckedAt: now.Format(time.RFC3339),
		}
		account := &Account{ID: 2, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: extra}
		paused, _ := shouldAutoPauseOpenAIAccountByQuota(context.Background(), account)
		require.False(t, paused)
	})

	t.Run("达到用卡阈值后即使有卡也退出调度", func(t *testing.T) {
		extra := cloneOpenAIAutoResetExtra(baseExtra)
		extra["codex_5h_used_percent"] = 100.0
		extra[OpenAIAutoResetCreditStateExtraKey] = OpenAIAutoResetCreditState{
			Status: OpenAIAutoResetStatusAvailable, AvailableCount: 1, CheckedAt: now.Format(time.RFC3339),
		}
		account := &Account{ID: 3, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: extra}
		paused, decision := shouldAutoPauseOpenAIAccountByQuota(context.Background(), account)
		require.True(t, paused)
		require.Equal(t, "quota_auto_reset_pending_5h", decision.reason)
	})

	t.Run("自然窗口重置后清除动态阻塞", func(t *testing.T) {
		extra := cloneOpenAIAutoResetExtra(baseExtra)
		extra["codex_5h_used_percent"] = 100.0
		extra["codex_5h_reset_at"] = now.Add(-time.Second).Format(time.RFC3339)
		extra[OpenAIAutoResetCreditStateExtraKey] = OpenAIAutoResetCreditState{
			Status: OpenAIAutoResetStatusFailed, TriggerWindow: "5h", ErrorCode: "RESET_FAILED",
		}
		account := &Account{ID: 4, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: extra}
		paused, _ := shouldAutoPauseOpenAIAccountByQuota(context.Background(), account)
		require.False(t, paused)
	})
}

func TestSelectOpenAIAutoResetCandidate_FailsClosed(t *testing.T) {
	candidates := []OpenAIRateLimitResetCreditDetail{
		{ID: "later", ExpiresAt: "2026-09-02T00:00:00Z"},
		{ID: "earlier", ExpiresAt: "2026-09-01T00:00:00Z"},
	}
	selected, err := selectOpenAIAutoResetCandidate(candidates, 2, nil, "cycle-a")
	require.NoError(t, err)
	require.Equal(t, "earlier", selected.ID)

	_, err = selectOpenAIAutoResetCandidate([]OpenAIRateLimitResetCreditDetail{
		{ExpiresAt: "2026-09-01T00:00:00Z"},
	}, 1, nil, "cycle-a")
	require.Error(t, err)

	_, err = selectOpenAIAutoResetCandidate(candidates, 2, &OpenAIAutoResetCreditState{
		AttemptCycleHash: "cycle-a", AttemptCreditHash: shortOpenAIAutoResetHash("missing"),
	}, "cycle-a")
	require.Error(t, err, "模糊结果后原卡消失时不得切换下一张卡")
}

func TestOpenAIQuotaAutoResetService_AssessesIndependentWindows(t *testing.T) {
	service := &OpenAIQuotaAutoResetService{}
	account := &Account{Extra: map[string]any{
		"auto_pause_5h_disabled": true,
		"auto_pause_7d_disabled": true,
	}}
	config := OpenAIAutoResetCreditConfig{Enabled: true, Threshold5h: 0.8, Threshold7d: 0.9}
	tests := []struct {
		name       string
		fiveHour   float64
		sevenDay   float64
		wantWindow string
	}{
		{name: "5h", fiveHour: 0.8, sevenDay: 0.2, wantWindow: "5h"},
		{name: "7d", fiveHour: 0.2, sevenDay: 0.9, wantWindow: "7d"},
		{name: "同时触发", fiveHour: 0.95, sevenDay: 0.95, wantWindow: "5h+7d"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assessment := service.buildAssessment(account, config, test.fiveHour, test.sevenDay)
			require.True(t, assessment.resetReached)
			require.Equal(t, test.wantWindow, assessment.triggerWindow)
		})
	}
}

type autoResetTestAccountRepo struct {
	AccountRepository
	mu      sync.Mutex
	account *Account
}

func (r *autoResetTestAccountRepo) GetByID(_ context.Context, id int64) (*Account, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	copy := *r.account
	copy.Extra = cloneOpenAIAutoResetExtra(r.account.Extra)
	return &copy, nil
}

func (r *autoResetTestAccountRepo) UpdateExtra(_ context.Context, id int64, updates map[string]any) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.account.Extra == nil {
		r.account.Extra = make(map[string]any)
	}
	for key, value := range updates {
		r.account.Extra[key] = value
	}
	return nil
}

func (r *autoResetTestAccountRepo) CompareAndSwapExtra(_ context.Context, _ int64, key string, expected any, updates map[string]any) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var current any
	if r.account.Extra != nil {
		current = r.account.Extra[key]
	}
	currentJSON, _ := json.Marshal(current)
	expectedJSON, _ := json.Marshal(expected)
	if string(currentJSON) != string(expectedJSON) {
		return false, nil
	}
	if r.account.Extra == nil {
		r.account.Extra = make(map[string]any)
	}
	for updateKey, value := range updates {
		r.account.Extra[updateKey] = value
	}
	return true, nil
}

type autoResetTestQuota struct {
	usage           *OpenAIQuotaUsage
	usageErr        error
	resetResult     *OpenAIQuotaResetResult
	usageQueryCalls atomic.Int32
	cacheCalls      atomic.Int32
	resetCalls      atomic.Int32
	resetEntered    chan struct{}
	releaseReset    chan struct{}
	enterOnce       sync.Once
	mu              sync.Mutex
	resetArgs       [][2]string
	failFirst       bool
}

func (q *autoResetTestQuota) QueryUsage(context.Context, int64) (*OpenAIQuotaUsage, error) {
	q.usageQueryCalls.Add(1)
	return q.queryResult()
}

func (q *autoResetTestQuota) queryResult() (*OpenAIQuotaUsage, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.usageErr != nil {
		return nil, q.usageErr
	}
	if q.usage == nil {
		return nil, nil
	}
	copy := *q.usage
	if q.usage.RateLimitResetCredits != nil {
		credits := *q.usage.RateLimitResetCredits
		credits.Credits = append([]OpenAIRateLimitResetCreditDetail(nil), credits.Credits...)
		copy.RateLimitResetCredits = &credits
	}
	return &copy, nil
}

func (q *autoResetTestQuota) CacheResetCreditsSnapshot(context.Context, int64, *OpenAIRateLimitResetCredits) error {
	q.cacheCalls.Add(1)
	return nil
}

func (q *autoResetTestQuota) ResetCreditTargeted(_ context.Context, _ int64, creditID, redeemRequestID string) (*OpenAIQuotaResetResult, error) {
	if creditID == "" || redeemRequestID == "" {
		panic("targeted reset identifiers must be present")
	}
	call := q.resetCalls.Add(1)
	q.mu.Lock()
	q.resetArgs = append(q.resetArgs, [2]string{creditID, redeemRequestID})
	resetResult := q.resetResult
	q.mu.Unlock()
	if q.failFirst && call == 1 {
		return nil, context.DeadlineExceeded
	}
	if q.resetEntered != nil {
		q.enterOnce.Do(func() { close(q.resetEntered) })
	}
	if q.releaseReset != nil {
		<-q.releaseReset
	}
	if resetResult != nil {
		copy := *resetResult
		return &copy, nil
	}
	return &OpenAIQuotaResetResult{Code: "ok", WindowsReset: 2}, nil
}

type autoResetTestRecoverer struct{}

func (autoResetTestRecoverer) RecoverAccountState(context.Context, int64, AccountRecoveryOptions) (*SuccessfulTestRecoveryResult, error) {
	return &SuccessfulTestRecoveryResult{ClearedRateLimit: true}, nil
}

func newAutoResetTestService(repo AccountRepository, quota openAIAutoResetQuota) *OpenAIQuotaAutoResetService {
	config := DefaultIdempotencyConfig()
	config.ObserveOnly = false
	return NewOpenAIQuotaAutoResetService(repo, quota, autoResetTestRecoverer{}, NewIdempotencyCoordinator(newInMemoryIdempotencyRepo(), config), nil, nil, nil)
}

func newExpiryTargetTestAccount(id int64, creditID, expiresAt string, leadTimeMinutes int, now time.Time) *Account {
	return &Account{
		ID: id, Platform: PlatformOpenAI, Type: AccountTypeOAuth,
		Status: StatusActive, Schedulable: true,
		Extra: map[string]any{
			OpenAIAutoResetCreditExpiryTargetExtraKey: OpenAIResetCreditExpiryTarget{
				PlanID: uuid.NewString(), CreditID: creditID, ExpiresAt: expiresAt, LeadTimeMinutes: leadTimeMinutes,
			},
			"codex_usage_updated_at": now.Add(-openAIAutoResetSnapshotTTL - time.Minute).Format(time.RFC3339),
		},
	}
}

func newExpiryTargetTestUsage(now time.Time, usedPercent float64, credits ...OpenAIRateLimitResetCreditDetail) *OpenAIQuotaUsage {
	return &OpenAIQuotaUsage{
		FetchedAt: now.Unix(),
		RateLimit: &OpenAIRateLimit{PrimaryWindow: &OpenAIRateLimitWindow{
			UsedPercent: usedPercent, LimitWindowSeconds: 5 * 60 * 60,
			ResetAfterSeconds: 3600, ResetAt: now.Add(time.Hour).Unix(),
		}},
		RateLimitResetCredits: &OpenAIRateLimitResetCredits{AvailableCount: len(credits), Credits: credits},
	}
}

func readAutoResetTestState(repo *autoResetTestAccountRepo) (*OpenAIAutoResetCreditState, *OpenAIResetCreditExpiryTarget) {
	repo.mu.Lock()
	defer repo.mu.Unlock()
	return openAIAutoResetStateFromExtra(repo.account.Extra), ResolveOpenAIResetCreditExpiryTarget(repo.account)
}

func newExpiryTargetTestFixture(account *Account, usage *OpenAIQuotaUsage) (*autoResetTestAccountRepo, *autoResetTestQuota, *OpenAIQuotaAutoResetService) {
	repo := &autoResetTestAccountRepo{account: account}
	quota := &autoResetTestQuota{usage: usage}
	return repo, quota, newAutoResetTestService(repo, quota)
}

func enableAutoResetThreshold(account *Account) {
	account.Extra[OpenAIAutoResetCreditEnabledExtraKey] = true
	account.Extra[OpenAIAutoResetCredit5hThresholdExtraKey] = 0.5
	account.Extra[OpenAIAutoResetCredit7dThresholdExtraKey] = 1.0
}

func autoResetTestResetArgs(quota *autoResetTestQuota) [][2]string {
	quota.mu.Lock()
	defer quota.mu.Unlock()
	return append([][2]string(nil), quota.resetArgs...)
}

func TestOpenAIQuotaAutoResetService_ConcurrentInstancesConsumeOnce(t *testing.T) {
	now := time.Now().UTC()
	account := &Account{
		ID: 99, Platform: PlatformOpenAI, Type: AccountTypeOAuth,
		Status: StatusActive, Schedulable: true,
		Extra: map[string]any{
			OpenAIAutoResetCreditEnabledExtraKey:     true,
			OpenAIAutoResetCredit5hThresholdExtraKey: 1.0,
			OpenAIAutoResetCredit7dThresholdExtraKey: 1.0,
			"codex_5h_used_percent":                  100.0,
			"codex_7d_used_percent":                  10.0,
			"codex_usage_updated_at":                 now.Format(time.RFC3339),
			"codex_5h_reset_at":                      now.Add(time.Hour).Format(time.RFC3339),
			"codex_7d_reset_at":                      now.Add(24 * time.Hour).Format(time.RFC3339),
		},
	}
	repo := &autoResetTestAccountRepo{account: account}
	usage := &OpenAIQuotaUsage{
		FetchedAt: now.Unix(),
		RateLimit: &OpenAIRateLimit{
			PrimaryWindow:   &OpenAIRateLimitWindow{UsedPercent: 100, LimitWindowSeconds: 5 * 60 * 60, ResetAfterSeconds: 3600, ResetAt: now.Add(time.Hour).Unix()},
			SecondaryWindow: &OpenAIRateLimitWindow{UsedPercent: 10, LimitWindowSeconds: 7 * 24 * 60 * 60, ResetAfterSeconds: 86400, ResetAt: now.Add(24 * time.Hour).Unix()},
		},
		RateLimitResetCredits: &OpenAIRateLimitResetCredits{
			AvailableCount: 1,
			Credits:        []OpenAIRateLimitResetCreditDetail{{ID: "credit-sensitive-id", ExpiresAt: now.Add(48 * time.Hour).Format(time.RFC3339)}},
		},
	}
	quota := &autoResetTestQuota{usage: usage, resetEntered: make(chan struct{}), releaseReset: make(chan struct{})}
	idempotencyRepo := newInMemoryIdempotencyRepo()
	config := DefaultIdempotencyConfig()
	config.ObserveOnly = false
	config.ProcessingTimeout = time.Second
	serviceA := NewOpenAIQuotaAutoResetService(repo, quota, autoResetTestRecoverer{}, NewIdempotencyCoordinator(idempotencyRepo, config), nil, nil, nil)
	serviceB := NewOpenAIQuotaAutoResetService(repo, quota, autoResetTestRecoverer{}, NewIdempotencyCoordinator(idempotencyRepo, config), nil, nil, nil)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_ = serviceA.evaluateAccount(context.Background(), account.ID)
	}()
	<-quota.resetEntered
	go func() {
		defer wg.Done()
		_ = serviceB.evaluateAccount(context.Background(), account.ID)
	}()
	time.Sleep(50 * time.Millisecond)
	close(quota.releaseReset)
	wg.Wait()

	require.Equal(t, int32(1), quota.resetCalls.Load())
	repo.mu.Lock()
	state := openAIAutoResetStateFromExtra(repo.account.Extra)
	repo.mu.Unlock()
	require.NotNil(t, state)
	require.Equal(t, OpenAIAutoResetStatusSuccess, state.Status)
	encodedState, err := json.Marshal(state)
	require.NoError(t, err)
	require.NotContains(t, string(encodedState), "credit-sensitive-id")
}

func TestOpenAIQuotaAutoResetService_TimeoutRetryReusesRequestBody(t *testing.T) {
	now := time.Now().UTC()
	account := &Account{
		ID: 100, Platform: PlatformOpenAI, Type: AccountTypeOAuth,
		Status: StatusActive, Schedulable: true,
		Extra: map[string]any{
			OpenAIAutoResetCreditEnabledExtraKey:     true,
			OpenAIAutoResetCredit5hThresholdExtraKey: 1.0,
			OpenAIAutoResetCredit7dThresholdExtraKey: 1.0,
			"codex_5h_used_percent":                  100.0,
			"codex_usage_updated_at":                 now.Format(time.RFC3339),
			"codex_5h_reset_at":                      now.Add(time.Hour).Format(time.RFC3339),
		},
	}
	repo := &autoResetTestAccountRepo{account: account}
	expiresAt := now.Add(48 * time.Hour).Format(time.RFC3339)
	quota := &autoResetTestQuota{
		failFirst: true,
		usage: &OpenAIQuotaUsage{
			FetchedAt: now.Unix(),
			RateLimit: &OpenAIRateLimit{
				PrimaryWindow: &OpenAIRateLimitWindow{UsedPercent: 100, LimitWindowSeconds: 5 * 60 * 60, ResetAfterSeconds: 3600, ResetAt: now.Add(time.Hour).Unix()},
			},
			RateLimitResetCredits: &OpenAIRateLimitResetCredits{
				AvailableCount: 1,
				Credits:        []OpenAIRateLimitResetCreditDetail{{ID: "retry-credit", ExpiresAt: expiresAt}},
			},
		},
	}
	idempotencyConfig := DefaultIdempotencyConfig()
	idempotencyConfig.ObserveOnly = false
	idempotencyConfig.FailedRetryBackoff = 0
	service := NewOpenAIQuotaAutoResetService(
		repo,
		quota,
		autoResetTestRecoverer{},
		NewIdempotencyCoordinator(newInMemoryIdempotencyRepo(), idempotencyConfig),
		nil, nil, nil,
	)

	require.Error(t, service.evaluateAccount(context.Background(), account.ID))
	require.NoError(t, service.evaluateAccount(context.Background(), account.ID))
	quota.mu.Lock()
	args := append([][2]string(nil), quota.resetArgs...)
	quota.mu.Unlock()
	require.Len(t, args, 2)
	require.Equal(t, args[0], args[1], "超时重试必须复用相同 credit_id 与 redeem_request_id")
}

func TestOpenAIQuotaAutoResetService_MissingCreditDetailsAreNotReportedAsWriteFailure(t *testing.T) {
	now := time.Now().UTC()
	account := &Account{
		ID: 126, Platform: PlatformOpenAI, Type: AccountTypeOAuth,
		Status: StatusActive, Schedulable: true,
		Extra: map[string]any{
			OpenAIAutoResetCreditEnabledExtraKey:     true,
			OpenAIAutoResetCredit5hThresholdExtraKey: 0.5,
			OpenAIAutoResetCredit7dThresholdExtraKey: 1.0,
			"codex_5h_used_percent":                  75.0,
			"codex_usage_updated_at":                 now.Format(time.RFC3339),
		},
	}
	repo := &autoResetTestAccountRepo{account: account}
	quota := &autoResetTestQuota{usage: &OpenAIQuotaUsage{
		FetchedAt: now.Unix(),
		RateLimit: &OpenAIRateLimit{PrimaryWindow: &OpenAIRateLimitWindow{
			UsedPercent: 75, LimitWindowSeconds: 5 * 60 * 60, ResetAt: now.Add(time.Hour).Unix(),
		}},
	}}

	require.Error(t, newAutoResetTestService(repo, quota).evaluateAccount(context.Background(), account.ID))
	state, _ := readAutoResetTestState(repo)
	require.Equal(t, "RESET_CREDIT_DETAILS_UNAVAILABLE", state.ErrorCode)
	require.Zero(t, quota.cacheCalls.Load())
	require.Zero(t, quota.resetCalls.Load())
}

func TestOpenAIQuotaAutoResetService_ExpiryTargetWaitsUntilScheduledTime(t *testing.T) {
	now := time.Now().UTC()
	expiresAt := now.Add(2 * time.Hour).Format(time.RFC3339)
	creditID := "future-expiry-target"
	account := newExpiryTargetTestAccount(121, creditID, expiresAt, 60, now)
	repo := &autoResetTestAccountRepo{account: account}
	quota := &autoResetTestQuota{}
	svc := newAutoResetTestService(repo, quota)

	require.NoError(t, svc.evaluateAccount(context.Background(), account.ID))
	require.Zero(t, quota.usageQueryCalls.Load())
	require.Zero(t, quota.resetCalls.Load())
	_, target := readAutoResetTestState(repo)
	require.NotNil(t, target)
}

func TestOpenAIQuotaAutoResetService_ExpiryTargetWinsWhenThresholdAlsoTriggers(t *testing.T) {
	now := time.Now().UTC()
	targetExpiry := now.Add(48 * time.Hour).Format(time.RFC3339)
	earlierExpiry := now.Add(24 * time.Hour).Format(time.RFC3339)
	targetID := "explicit-expiry-target"
	account := newExpiryTargetTestAccount(106, targetID, targetExpiry, 3*24*60, now)
	enableAutoResetThreshold(account)
	repo := &autoResetTestAccountRepo{account: account}
	quota := &autoResetTestQuota{usage: newExpiryTargetTestUsage(now, 75,
		OpenAIRateLimitResetCreditDetail{ID: "earlier-threshold-credit", ExpiresAt: earlierExpiry},
		OpenAIRateLimitResetCreditDetail{ID: targetID, ExpiresAt: targetExpiry},
	)}
	svc := newAutoResetTestService(repo, quota)

	require.NoError(t, svc.evaluateAccount(context.Background(), account.ID))
	require.Equal(t, int32(1), quota.usageQueryCalls.Load(), "临期计划直接执行，只在消费后公共收尾回读一次")
	require.Equal(t, targetID, autoResetTestResetArgs(quota)[0][0])
}

func TestOpenAIQuotaAutoResetService_ThresholdCanConsumePlannedCreditBeforeExpiryWindow(t *testing.T) {
	now := time.Now().UTC()
	targetID := "planned-credit"
	targetExpiry := now.Add(72 * time.Hour).Format(time.RFC3339)
	account := newExpiryTargetTestAccount(110, targetID, targetExpiry, 60, now)
	enableAutoResetThreshold(account)
	repo, quota, svc := newExpiryTargetTestFixture(account, newExpiryTargetTestUsage(now, 75,
		OpenAIRateLimitResetCreditDetail{ID: targetID, ExpiresAt: targetExpiry},
		OpenAIRateLimitResetCreditDetail{ID: "later-credit", ExpiresAt: now.Add(96 * time.Hour).Format(time.RFC3339)},
	))

	require.NoError(t, svc.evaluateAccount(context.Background(), account.ID))
	require.Equal(t, targetID, autoResetTestResetArgs(quota)[0][0])
	_, target := readAutoResetTestState(repo)
	require.Nil(t, target)
}

func TestOpenAIQuotaAutoResetService_ExpiryTargetConsumesExactCreditWithoutUsageDependency(t *testing.T) {
	now := time.Now().UTC()
	expiresAt := now.Add(30 * time.Minute).Format(time.RFC3339)
	creditID := "unused-expiry-target"
	account := newExpiryTargetTestAccount(102, creditID, expiresAt, OpenAIResetCreditExpiryTargetDefaultLeadTimeMinutes, now)
	account.Extra["codex_usage_updated_at"] = now.Format(time.RFC3339)
	account.Extra["codex_5h_used_percent"] = 0.0
	repo, quota, svc := newExpiryTargetTestFixture(account, newExpiryTargetTestUsage(now, 0,
		OpenAIRateLimitResetCreditDetail{ID: "other-credit", ExpiresAt: now.Add(10 * time.Minute).Format(time.RFC3339)},
		OpenAIRateLimitResetCreditDetail{ID: creditID, ExpiresAt: expiresAt},
	))

	require.NoError(t, svc.evaluateAccount(context.Background(), account.ID))
	require.Equal(t, int32(1), quota.usageQueryCalls.Load(), "纯定时执行前不得请求完整用量，消费后公共收尾仍回读一次")
	require.Equal(t, int32(1), quota.cacheCalls.Load(), "执行前不写快照，消费后公共收尾回写一次")
	require.Equal(t, int32(1), quota.resetCalls.Load())
	require.Equal(t, creditID, autoResetTestResetArgs(quota)[0][0])
	state, target := readAutoResetTestState(repo)
	require.Nil(t, target)
	require.Equal(t, OpenAIAutoResetStatusSuccess, state.Status)
	require.Equal(t, OpenAIAutoResetTriggerReasonExpiryTarget, state.TriggerReason)
	require.Empty(t, state.ErrorCode)
}

func TestOpenAIQuotaAutoResetService_ReplayedCreditStillAvailableFails(t *testing.T) {
	now := time.Now().UTC()
	expiresAt := now.Add(30 * time.Minute).Format(time.RFC3339)
	creditID := "replayed-still-available"
	account := newExpiryTargetTestAccount(128, creditID, expiresAt, OpenAIResetCreditExpiryTargetDefaultLeadTimeMinutes, now)
	repo, quota, svc := newExpiryTargetTestFixture(account, newExpiryTargetTestUsage(now, 0,
		OpenAIRateLimitResetCreditDetail{ID: creditID, ExpiresAt: expiresAt},
	))

	require.NoError(t, svc.evaluateAccount(context.Background(), account.ID))
	require.NoError(t, repo.UpdateExtra(context.Background(), account.ID, map[string]any{
		OpenAIAutoResetCreditExpiryTargetExtraKey: OpenAIResetCreditExpiryTarget{
			PlanID: uuid.NewString(), CreditID: creditID, ExpiresAt: expiresAt,
			LeadTimeMinutes: OpenAIResetCreditExpiryTargetDefaultLeadTimeMinutes,
		},
	}))

	require.NoError(t, svc.evaluateAccount(context.Background(), account.ID))
	state, target := readAutoResetTestState(repo)
	require.Nil(t, target)
	require.Equal(t, OpenAIAutoResetStatusFailed, state.Status)
	require.Equal(t, openAIAutoResetReplayConflictCode, state.ErrorCode)
	require.Equal(t, 1, state.AvailableCount)
	require.Equal(t, int32(1), quota.resetCalls.Load(), "历史回放不得再次调用上游")
	require.Equal(t, int32(2), quota.usageQueryCalls.Load(), "回放必须先额外核验实时库存")
}

func TestOpenAIQuotaAutoResetService_ReplayedMissingCreditCompletesFinalization(t *testing.T) {
	now := time.Now().UTC()
	expiresAt := now.Add(30 * time.Minute).Format(time.RFC3339)
	creditID := "replayed-consumed-credit"
	account := newExpiryTargetTestAccount(129, creditID, expiresAt, OpenAIResetCreditExpiryTargetDefaultLeadTimeMinutes, now)
	repo, quota, svc := newExpiryTargetTestFixture(account, newExpiryTargetTestUsage(now, 0,
		OpenAIRateLimitResetCreditDetail{ID: creditID, ExpiresAt: expiresAt},
	))

	require.NoError(t, svc.evaluateAccount(context.Background(), account.ID))
	quota.mu.Lock()
	quota.usage = newExpiryTargetTestUsage(now, 0)
	quota.mu.Unlock()
	require.NoError(t, repo.UpdateExtra(context.Background(), account.ID, map[string]any{
		OpenAIAutoResetCreditExpiryTargetExtraKey: OpenAIResetCreditExpiryTarget{
			PlanID: uuid.NewString(), CreditID: creditID, ExpiresAt: expiresAt,
			LeadTimeMinutes: OpenAIResetCreditExpiryTargetDefaultLeadTimeMinutes,
		},
	}))

	require.NoError(t, svc.evaluateAccount(context.Background(), account.ID))
	state, target := readAutoResetTestState(repo)
	require.Nil(t, target)
	require.Equal(t, OpenAIAutoResetStatusSuccess, state.Status)
	require.Empty(t, state.ErrorCode)
	require.Zero(t, state.AvailableCount)
	require.Equal(t, int32(1), quota.resetCalls.Load())
	require.Equal(t, int32(3), quota.usageQueryCalls.Load(), "回放核验后仍执行公共收尾")
}

func TestOpenAIQuotaAutoResetService_ReplayVerificationFailureKeepsPlan(t *testing.T) {
	now := time.Now().UTC()
	expiresAt := now.Add(30 * time.Minute).Format(time.RFC3339)
	creditID := "replay-unverified-credit"
	account := newExpiryTargetTestAccount(130, creditID, expiresAt, OpenAIResetCreditExpiryTargetDefaultLeadTimeMinutes, now)
	repo, quota, svc := newExpiryTargetTestFixture(account, newExpiryTargetTestUsage(now, 0,
		OpenAIRateLimitResetCreditDetail{ID: creditID, ExpiresAt: expiresAt},
	))

	require.NoError(t, svc.evaluateAccount(context.Background(), account.ID))
	replacement := OpenAIResetCreditExpiryTarget{
		PlanID: uuid.NewString(), CreditID: creditID, ExpiresAt: expiresAt,
		LeadTimeMinutes: OpenAIResetCreditExpiryTargetDefaultLeadTimeMinutes,
	}
	require.NoError(t, repo.UpdateExtra(context.Background(), account.ID, map[string]any{
		OpenAIAutoResetCreditExpiryTargetExtraKey: replacement,
	}))
	quota.mu.Lock()
	quota.usage = &OpenAIQuotaUsage{
		FetchedAt:             now.Unix(),
		RateLimitResetCredits: &OpenAIRateLimitResetCredits{AvailableCount: 1},
	}
	quota.mu.Unlock()

	require.Error(t, svc.evaluateAccount(context.Background(), account.ID))
	state, target := readAutoResetTestState(repo)
	require.Equal(t, &replacement, target)
	require.Equal(t, OpenAIAutoResetStatusFailed, state.Status)
	require.Equal(t, "OPENAI_AUTO_RESET_CREDIT_DETAILS_INCOMPLETE", state.ErrorCode)
	require.Equal(t, int32(1), quota.resetCalls.Load())
}

func TestOpenAIQuotaAutoResetService_ReplayVerificationQueryFailureKeepsPlan(t *testing.T) {
	now := time.Now().UTC()
	expiresAt := now.Add(30 * time.Minute).Format(time.RFC3339)
	creditID := "replay-query-failed-credit"
	account := newExpiryTargetTestAccount(133, creditID, expiresAt, OpenAIResetCreditExpiryTargetDefaultLeadTimeMinutes, now)
	repo, quota, svc := newExpiryTargetTestFixture(account, newExpiryTargetTestUsage(now, 0,
		OpenAIRateLimitResetCreditDetail{ID: creditID, ExpiresAt: expiresAt},
	))

	require.NoError(t, svc.evaluateAccount(context.Background(), account.ID))
	replacement := OpenAIResetCreditExpiryTarget{
		PlanID: uuid.NewString(), CreditID: creditID, ExpiresAt: expiresAt,
		LeadTimeMinutes: OpenAIResetCreditExpiryTargetDefaultLeadTimeMinutes,
	}
	require.NoError(t, repo.UpdateExtra(context.Background(), account.ID, map[string]any{
		OpenAIAutoResetCreditExpiryTargetExtraKey: replacement,
	}))
	quota.mu.Lock()
	quota.usageErr = context.DeadlineExceeded
	quota.mu.Unlock()

	require.Error(t, svc.evaluateAccount(context.Background(), account.ID))
	state, target := readAutoResetTestState(repo)
	require.Equal(t, &replacement, target)
	require.Equal(t, OpenAIAutoResetStatusFailed, state.Status)
	require.Equal(t, "RESET_CREDIT_QUERY_FAILED", state.ErrorCode)
	require.Equal(t, int32(1), quota.resetCalls.Load())
}

func TestOpenAIQuotaAutoResetService_ZeroWindowResultUsesInventory(t *testing.T) {
	now := time.Now().UTC()
	expiresAt := now.Add(30 * time.Minute).Format(time.RFC3339)

	t.Run("卡仍存在时不得成功", func(t *testing.T) {
		creditID := "zero-window-still-present"
		account := newExpiryTargetTestAccount(131, creditID, expiresAt, OpenAIResetCreditExpiryTargetDefaultLeadTimeMinutes, now)
		repo, quota, svc := newExpiryTargetTestFixture(account, newExpiryTargetTestUsage(now, 0,
			OpenAIRateLimitResetCreditDetail{ID: creditID, ExpiresAt: expiresAt},
		))
		quota.resetResult = &OpenAIQuotaResetResult{Code: "ok", WindowsReset: 0}

		require.NoError(t, svc.evaluateAccount(context.Background(), account.ID))
		state, target := readAutoResetTestState(repo)
		require.Nil(t, target)
		require.Equal(t, OpenAIAutoResetStatusFailed, state.Status)
		require.Equal(t, openAIAutoResetNoEffectCode, state.ErrorCode)
		require.Equal(t, 1, state.AvailableCount)
	})

	t.Run("卡已消失时允许成功", func(t *testing.T) {
		creditID := "zero-window-consumed"
		account := newExpiryTargetTestAccount(132, creditID, expiresAt, OpenAIResetCreditExpiryTargetDefaultLeadTimeMinutes, now)
		repo, quota, svc := newExpiryTargetTestFixture(account, newExpiryTargetTestUsage(now, 0))
		quota.resetResult = &OpenAIQuotaResetResult{Code: "ok", WindowsReset: 0}

		require.NoError(t, svc.evaluateAccount(context.Background(), account.ID))
		state, target := readAutoResetTestState(repo)
		require.Nil(t, target)
		require.Equal(t, OpenAIAutoResetStatusSuccess, state.Status)
		require.Empty(t, state.ErrorCode)
		require.Zero(t, state.AvailableCount)
		require.Equal(t, int32(2), quota.usageQueryCalls.Load())
	})
}

func TestOpenAIQuotaAutoResetService_TerminalReplayFailureDoesNotRepeatWithinCycle(t *testing.T) {
	now := time.Now().UTC()
	creditID := "threshold-replay-conflict"
	expiresAt := now.Add(24 * time.Hour).Format(time.RFC3339)
	account := &Account{
		ID: 134, Platform: PlatformOpenAI, Type: AccountTypeOAuth,
		Status: StatusActive, Schedulable: true,
		Extra: map[string]any{
			OpenAIAutoResetCreditEnabledExtraKey:     true,
			OpenAIAutoResetCredit5hThresholdExtraKey: 0.5,
			OpenAIAutoResetCredit7dThresholdExtraKey: 1.0,
			"codex_5h_used_percent":                  75.0,
			"codex_usage_updated_at":                 now.Format(time.RFC3339),
			"codex_5h_reset_at":                      now.Add(time.Hour).Format(time.RFC3339),
		},
	}
	repo, quota, svc := newExpiryTargetTestFixture(account, newExpiryTargetTestUsage(now, 75,
		OpenAIRateLimitResetCreditDetail{ID: creditID, ExpiresAt: expiresAt},
	))

	require.NoError(t, svc.evaluateAccount(context.Background(), account.ID))
	require.NoError(t, svc.evaluateAccount(context.Background(), account.ID))
	state, _ := readAutoResetTestState(repo)
	require.Equal(t, OpenAIAutoResetStatusFailed, state.Status)
	require.Equal(t, openAIAutoResetReplayConflictCode, state.ErrorCode)
	require.Equal(t, int32(1), quota.resetCalls.Load())
	require.Equal(t, int32(4), quota.usageQueryCalls.Load())

	require.NoError(t, svc.evaluateAccount(context.Background(), account.ID))
	state, _ = readAutoResetTestState(repo)
	require.Equal(t, OpenAIAutoResetStatusFailed, state.Status)
	require.Equal(t, openAIAutoResetReplayConflictCode, state.ErrorCode)
	require.Equal(t, int32(1), quota.resetCalls.Load())
	require.Equal(t, int32(5), quota.usageQueryCalls.Load(), "同一周期只刷新一次库存，不再重复应用历史结果")
}

func TestOpenAIQuotaAutoResetService_StaleExpiryWorkerCannotOverwriteReplacementState(t *testing.T) {
	now := time.Now().UTC()
	oldTarget := &OpenAIResetCreditExpiryTarget{
		PlanID: uuid.NewString(), CreditID: "old-credit",
		ExpiresAt: now.Add(time.Hour).Format(time.RFC3339), LeadTimeMinutes: 60,
	}
	replacement := OpenAIResetCreditExpiryTarget{
		PlanID: uuid.NewString(), CreditID: "replacement-credit",
		ExpiresAt: now.Add(2 * time.Hour).Format(time.RFC3339), LeadTimeMinutes: 60,
	}
	existingState := OpenAIAutoResetCreditState{
		Status: OpenAIAutoResetStatusAvailable, AvailableCount: 2,
		CheckedAt: now.Format(time.RFC3339),
	}
	account := &Account{
		ID: 127, Platform: PlatformOpenAI, Type: AccountTypeOAuth,
		Status: StatusActive, Schedulable: true,
		Extra: map[string]any{
			OpenAIAutoResetCreditExpiryTargetExtraKey: replacement,
			OpenAIAutoResetCreditStateExtraKey:        existingState,
		},
	}
	repo, quota, svc := newExpiryTargetTestFixture(account, nil)

	require.NoError(t, svc.consumeOpenAIAutoResetCredit(
		context.Background(), account.ID, oldTarget.CreditID,
		OpenAIAutoResetTriggerReasonExpiryTarget, oldTarget,
		openAIAutoResetAssessment{}, existingState.AvailableCount, "",
	))

	state, target := readAutoResetTestState(repo)
	require.Equal(t, &replacement, target)
	require.Equal(t, &existingState, state)
	require.Zero(t, quota.resetCalls.Load())
}

func TestOpenAIQuotaAutoResetService_ExpiryTargetRetryKeepsRedeemIDAcrossQuotaCycles(t *testing.T) {
	now := time.Now().UTC()
	expiresAt := now.Add(30 * time.Minute).Format(time.RFC3339)
	creditID := "expiry-retry-target"
	account := newExpiryTargetTestAccount(123, creditID, expiresAt, OpenAIResetCreditExpiryTargetDefaultLeadTimeMinutes, now)
	repo := &autoResetTestAccountRepo{account: account}
	quota := &autoResetTestQuota{
		failFirst: true,
		usage:     newExpiryTargetTestUsage(now, 0, OpenAIRateLimitResetCreditDetail{ID: creditID, ExpiresAt: expiresAt}),
	}
	idempotencyConfig := DefaultIdempotencyConfig()
	idempotencyConfig.ObserveOnly = false
	idempotencyConfig.FailedRetryBackoff = 0
	svc := NewOpenAIQuotaAutoResetService(
		repo,
		quota,
		autoResetTestRecoverer{},
		NewIdempotencyCoordinator(newInMemoryIdempotencyRepo(), idempotencyConfig),
		nil, nil, nil,
	)

	require.ErrorIs(t, svc.evaluateAccount(context.Background(), account.ID), context.DeadlineExceeded)
	quota.usage.RateLimit.PrimaryWindow.ResetAt = now.Add(2 * time.Hour).Unix()
	require.NoError(t, svc.evaluateAccount(context.Background(), account.ID))
	args := autoResetTestResetArgs(quota)
	require.Len(t, args, 2)
	require.Equal(t, args[0], args[1], "定时计划跨配额周期重试仍须复用相同 redeem_request_id")
}

func TestOpenAIQuotaAutoResetService_ExpiryTargetEndsExpiredPlan(t *testing.T) {
	now := time.Now().UTC()
	account := newExpiryTargetTestAccount(104, "expired-credit", now.Add(-time.Minute).Format(time.RFC3339), OpenAIResetCreditExpiryTargetDefaultLeadTimeMinutes, now)
	repo, quota, svc := newExpiryTargetTestFixture(account, nil)

	require.NoError(t, svc.evaluateAccount(context.Background(), account.ID))
	require.Zero(t, quota.usageQueryCalls.Load())
	require.Zero(t, quota.resetCalls.Load())
	state, target := readAutoResetTestState(repo)
	require.Nil(t, target)
	require.Equal(t, OpenAIAutoResetStatusFailed, state.Status)
	require.Equal(t, OpenAIAutoResetTriggerReasonExpiryTarget, state.TriggerReason)
	require.Equal(t, "OPENAI_RESET_CREDIT_EXPIRED_UNUSED", state.ErrorCode)
	require.NotEmpty(t, state.LastResultAt)
}

func TestOpenAIQuotaAutoResetService_ExpiryTargetUsesIndependentClaimAfterThreshold(t *testing.T) {
	now := time.Now().UTC()
	targetID := "independent-planned-credit"
	otherID := "independent-threshold-credit"
	targetExpiry := now.Add(72 * time.Hour).Format(time.RFC3339)
	otherExpiry := now.Add(24 * time.Hour).Format(time.RFC3339)
	account := newExpiryTargetTestAccount(125, targetID, targetExpiry, 60, now)
	enableAutoResetThreshold(account)
	repo := &autoResetTestAccountRepo{account: account}
	quota := &autoResetTestQuota{usage: newExpiryTargetTestUsage(now, 75,
		OpenAIRateLimitResetCreditDetail{ID: otherID, ExpiresAt: otherExpiry},
		OpenAIRateLimitResetCreditDetail{ID: targetID, ExpiresAt: targetExpiry},
	)}
	svc := newAutoResetTestService(repo, quota)

	require.NoError(t, svc.evaluateAccount(context.Background(), account.ID))
	repo.mu.Lock()
	target := ResolveOpenAIResetCreditExpiryTarget(repo.account)
	repo.mu.Unlock()
	require.NotNil(t, target)
	target.LeadTimeMinutes = OpenAIResetCreditExpiryTargetMaxLeadTimeMinutes
	repo.mu.Lock()
	repo.account.Extra[OpenAIAutoResetCreditExpiryTargetExtraKey] = target
	repo.mu.Unlock()
	quota.usage = newExpiryTargetTestUsage(now, 0, OpenAIRateLimitResetCreditDetail{ID: targetID, ExpiresAt: targetExpiry})

	require.NoError(t, svc.evaluateAccount(context.Background(), account.ID))
	args := autoResetTestResetArgs(quota)
	require.Len(t, args, 2)
	require.Equal(t, otherID, args[0][0])
	require.Equal(t, targetID, args[1][0])
}
