package service

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

func TestNormalizeOpenAIAutoResetCreditExtra(t *testing.T) {
	t.Run("历史账号默认关闭", func(t *testing.T) {
		account := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}
		config := ResolveOpenAIAutoResetCreditConfig(account)
		require.False(t, config.Enabled)
		require.Equal(t, 0.0, config.Threshold5h, "5h 默认关闭")
		require.Equal(t, 1.0, config.Threshold7d)
		require.False(t, config.ExpiryEnabled)
		require.False(t, config.Active())
		require.Equal(t, 10*time.Minute, config.ExpiryLead)
	})

	t.Run("到期用卡独立开关", func(t *testing.T) {
		extra, err := normalizeOpenAIAutoResetCreditExtra(PlatformOpenAI, AccountTypeOAuth, false, map[string]any{
			OpenAIAutoResetCreditExpiryEnabledExtraKey: true,
		})
		require.NoError(t, err)
		require.Equal(t, 10.0, extra[OpenAIAutoResetCreditExpiryLeadMinutesExtraKey], "开启时补齐默认 10 分钟")
		config := ResolveOpenAIAutoResetCreditConfig(&Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: extra})
		require.False(t, config.Enabled)
		require.True(t, config.ExpiryEnabled)
		require.True(t, config.Active())
		require.Equal(t, 10*time.Minute, config.ExpiryLead)

		_, err = normalizeOpenAIAutoResetCreditExtra(PlatformOpenAI, AccountTypeOAuth, false, map[string]any{
			OpenAIAutoResetCreditExpiryEnabledExtraKey:     true,
			OpenAIAutoResetCreditExpiryLeadMinutesExtraKey: 0,
		})
		require.Error(t, err, "开启到期用卡时提前量不能为 0")

		_, err = normalizeOpenAIAutoResetCreditExtra(PlatformOpenAI, AccountTypeOAuth, false, map[string]any{
			OpenAIAutoResetCreditExpiryEnabledExtraKey: "yes",
		})
		require.Error(t, err)

		stripped := stripOpenAIAutoResetCreditManagedExtra(map[string]any{OpenAIAutoResetCreditExpiryEnabledExtraKey: true}, true)
		require.NotContains(t, stripped, OpenAIAutoResetCreditExpiryEnabledExtraKey)
	})

	t.Run("到期提前量为非负整数分钟", func(t *testing.T) {
		extra, err := normalizeOpenAIAutoResetCreditExtra(PlatformOpenAI, AccountTypeOAuth, false, map[string]any{
			OpenAIAutoResetCreditEnabledExtraKey:           true,
			OpenAIAutoResetCreditExpiryLeadMinutesExtraKey: "1440",
		})
		require.NoError(t, err)
		require.Equal(t, 1440.0, extra[OpenAIAutoResetCreditExpiryLeadMinutesExtraKey])
		config := ResolveOpenAIAutoResetCreditConfig(&Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: extra})
		require.Equal(t, 24*time.Hour, config.ExpiryLead)

		extra, err = normalizeOpenAIAutoResetCreditExtra(PlatformOpenAI, AccountTypeOAuth, false, map[string]any{
			OpenAIAutoResetCreditEnabledExtraKey:           true,
			OpenAIAutoResetCreditExpiryLeadMinutesExtraKey: 10,
		})
		require.NoError(t, err, "非零提前量的下限是 10 分钟")
		require.Equal(t, 10.0, extra[OpenAIAutoResetCreditExpiryLeadMinutesExtraKey])

		for _, invalid := range []any{-1, 1.5, 5, "abc"} {
			_, err := normalizeOpenAIAutoResetCreditExtra(PlatformOpenAI, AccountTypeOAuth, false, map[string]any{
				OpenAIAutoResetCreditEnabledExtraKey:           true,
				OpenAIAutoResetCreditExpiryLeadMinutesExtraKey: invalid,
			})
			require.Error(t, err, "%v", invalid)
		}

		stripped := stripOpenAIAutoResetCreditManagedExtra(map[string]any{OpenAIAutoResetCreditExpiryLeadMinutesExtraKey: 60}, true)
		require.NotContains(t, stripped, OpenAIAutoResetCreditExpiryLeadMinutesExtraKey)
	})

	t.Run("开启时补齐默认阈值（5h 关闭、7d 百分百）并剥离运行态", func(t *testing.T) {
		extra, err := normalizeOpenAIAutoResetCreditExtra(PlatformOpenAI, AccountTypeOAuth, false, map[string]any{
			OpenAIAutoResetCreditEnabledExtraKey: true,
			OpenAIAutoResetCreditStateExtraKey:   map[string]any{"status": "success"},
		})
		require.NoError(t, err)
		require.Equal(t, 0.0, extra[OpenAIAutoResetCredit5hThresholdExtraKey])
		require.Equal(t, 1.0, extra[OpenAIAutoResetCredit7dThresholdExtraKey])
		require.NotContains(t, extra, OpenAIAutoResetCreditStateExtraKey)
	})

	t.Run("阈值和账号类型严格校验", func(t *testing.T) {
		extra, err := normalizeOpenAIAutoResetCreditExtra(PlatformOpenAI, AccountTypeOAuth, false, map[string]any{
			OpenAIAutoResetCreditEnabledExtraKey:     true,
			OpenAIAutoResetCredit7dThresholdExtraKey: 0,
		})
		require.NoError(t, err, "0 表示该窗口不触发，是合法值")
		require.Equal(t, 0.0, extra[OpenAIAutoResetCredit7dThresholdExtraKey])

		_, err = normalizeOpenAIAutoResetCreditExtra(PlatformOpenAI, AccountTypeOAuth, false, map[string]any{
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

	t.Run("5h 阈值为 0 时用满也不按待用卡暂停", func(t *testing.T) {
		extra := cloneOpenAIAutoResetExtra(baseExtra)
		extra[OpenAIAutoResetCredit5hThresholdExtraKey] = 0.0
		extra["codex_5h_used_percent"] = 100.0
		extra[OpenAIAutoResetCreditStateExtraKey] = OpenAIAutoResetCreditState{
			Status: OpenAIAutoResetStatusAvailable, AvailableCount: 1, CheckedAt: now.Format(time.RFC3339),
		}
		account := &Account{ID: 5, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: extra}
		paused, decision := shouldAutoPauseOpenAIAccountByQuota(context.Background(), account)
		require.False(t, paused)
		require.NotEqual(t, "quota_auto_reset_pending_5h", decision.reason)
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
	candidates := []openAIAutoResetCreditCandidate{
		{ID: "later", ExpiresAt: "2026-09-02T00:00:00Z"},
		{ID: "earlier", ExpiresAt: "2026-09-01T00:00:00Z"},
	}
	selected, err := selectOpenAIAutoResetCandidate(candidates, 2, nil, "cycle-a")
	require.NoError(t, err)
	require.Equal(t, "earlier", selected.ID)

	_, err = selectOpenAIAutoResetCandidate([]openAIAutoResetCreditCandidate{
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
			assessment := service.buildAssessment(account, config, test.fiveHour, test.sevenDay, false)
			require.True(t, assessment.resetReached)
			require.Equal(t, test.wantWindow, assessment.triggerWindow)
		})
	}

	disabled5h := OpenAIAutoResetCreditConfig{Enabled: true, Threshold5h: 0, Threshold7d: 0.9}
	assessment := service.buildAssessment(account, disabled5h, 1, 0.2, false)
	require.False(t, assessment.resetReached, "5h 阈值为 0 时用满也不触发")
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

func (r *autoResetTestAccountRepo) ListWithFilters(context.Context, pagination.PaginationParams, string, string, string, string, int64, string) ([]Account, *pagination.PaginationResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return []Account{*r.account}, &pagination.PaginationResult{Pages: 1}, nil
}

type autoResetTestQuota struct {
	usage        *OpenAIQuotaUsage
	resetCalls   atomic.Int32
	queryCalls   atomic.Int32
	cacheCalls   atomic.Int32
	cacheErr     error
	queryErr     error
	resetEntered chan struct{}
	releaseReset chan struct{}
	enterOnce    sync.Once
	mu           sync.Mutex
	resetArgs    [][2]string
	failFirst    bool
}

func (q *autoResetTestQuota) QueryUsage(context.Context, int64) (*OpenAIQuotaUsage, error) {
	q.queryCalls.Add(1)
	if q.queryErr != nil {
		return nil, q.queryErr
	}
	copy := *q.usage
	return &copy, nil
}

func (q *autoResetTestQuota) CacheResetCreditsSnapshot(context.Context, int64, *OpenAIRateLimitResetCredits) error {
	q.cacheCalls.Add(1)
	return q.cacheErr
}

func (q *autoResetTestQuota) CachePostResetSnapshot(context.Context, int64, *OpenAIQuotaUsage) error {
	return nil
}

func (q *autoResetTestQuota) ResetCreditTargeted(_ context.Context, _ int64, creditID, redeemRequestID string) (*OpenAIQuotaResetResult, error) {
	if creditID == "" || redeemRequestID == "" {
		panic("targeted reset identifiers must be present")
	}
	call := q.resetCalls.Add(1)
	q.mu.Lock()
	q.resetArgs = append(q.resetArgs, [2]string{creditID, redeemRequestID})
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
	return &OpenAIQuotaResetResult{Code: "ok", WindowsReset: 2}, nil
}

type autoResetTestRecoverer struct{}

func (autoResetTestRecoverer) RecoverAccountState(context.Context, int64, AccountRecoveryOptions) (*SuccessfulTestRecoveryResult, error) {
	return &SuccessfulTestRecoveryResult{ClearedRateLimit: true}, nil
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
			Credits:        []OpenAIRateLimitResetCreditDetail{{ExpiresAt: now.Add(48 * time.Hour).Format(time.RFC3339)}},
		},
		autoResetCandidates: []openAIAutoResetCreditCandidate{{ID: "credit-sensitive-id", ExpiresAt: now.Add(48 * time.Hour).Format(time.RFC3339)}},
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
				Credits:        []OpenAIRateLimitResetCreditDetail{{ExpiresAt: expiresAt}},
			},
			autoResetCandidates: []openAIAutoResetCreditCandidate{{ID: "retry-credit", ExpiresAt: expiresAt}},
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

func TestOpenAIQuotaAutoResetService_ExpiryTrigger(t *testing.T) {
	now := time.Now().UTC()
	in := func(d time.Duration) string { return now.Add(d).Format(time.RFC3339) }
	tests := []struct {
		name        string
		lead        time.Duration
		expirations []string
		want        bool
	}{
		{name: "未配置提前量不触发", lead: 0, expirations: []string{in(time.Minute)}, want: false},
		{name: "窗内触发", lead: time.Hour, expirations: []string{in(48 * time.Hour), in(30 * time.Minute)}, want: true},
		{name: "窗外不触发", lead: time.Hour, expirations: []string{in(90 * time.Minute)}, want: false},
		{name: "已过期不触发", lead: time.Hour, expirations: []string{in(-time.Minute)}, want: false},
		{name: "无法解析跳过", lead: time.Hour, expirations: []string{"bad", ""}, want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.want, openAIAutoResetCreditExpiring(test.expirations, test.lead, now))
		})
	}

	service := &OpenAIQuotaAutoResetService{}
	account := &Account{Extra: map[string]any{
		"auto_pause_5h_disabled": true,
		"auto_pause_7d_disabled": true,
	}}
	config := OpenAIAutoResetCreditConfig{Enabled: true, Threshold5h: 1, Threshold7d: 1, ExpiryLead: time.Hour}
	assessment := service.buildAssessment(account, config, 0.1, 0.1, true)
	require.True(t, assessment.resetReached)
	require.Equal(t, "expiry", assessment.triggerWindow)
	assessment = service.buildAssessment(account, config, 1, 0.1, true)
	require.Equal(t, "5h+expiry", assessment.triggerWindow)
}

func TestOpenAIAutoResetEarliestExpiryDelay(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	in := func(d time.Duration) string { return now.Add(d).Format(time.RFC3339) }
	_, ok := openAIAutoResetEarliestExpiryDelay([]string{in(time.Hour)}, 0, now)
	require.False(t, ok, "未配置提前量不排定时器")
	_, ok = openAIAutoResetEarliestExpiryDelay([]string{in(-time.Minute), "bad"}, time.Hour, now)
	require.False(t, ok, "没有未到期的卡不排定时器")
	delay, ok := openAIAutoResetEarliestExpiryDelay([]string{in(72 * time.Hour), in(26 * time.Hour)}, 24*time.Hour, now)
	require.True(t, ok)
	require.Equal(t, 2*time.Hour, delay.Round(time.Second), "以最早到期的卡计算")
}

func newAutoResetTestService(repo AccountRepository, quota openAIAutoResetQuota) *OpenAIQuotaAutoResetService {
	config := DefaultIdempotencyConfig()
	config.ObserveOnly = false
	config.ProcessingTimeout = time.Second
	return NewOpenAIQuotaAutoResetService(repo, quota, autoResetTestRecoverer{}, NewIdempotencyCoordinator(newInMemoryIdempotencyRepo(), config), nil, nil, nil)
}

func newAutoResetLowUsageAccount(now time.Time, extra map[string]any) *Account {
	account := &Account{
		ID: 7, Platform: PlatformOpenAI, Type: AccountTypeOAuth,
		Status: StatusActive, Schedulable: true,
		Extra: map[string]any{
			OpenAIAutoResetCreditEnabledExtraKey:     true,
			OpenAIAutoResetCredit5hThresholdExtraKey: 1.0,
			OpenAIAutoResetCredit7dThresholdExtraKey: 1.0,
			"codex_5h_used_percent":                  10.0,
			"codex_7d_used_percent":                  10.0,
			"codex_usage_updated_at":                 now.Format(time.RFC3339),
			"codex_5h_reset_at":                      now.Add(time.Hour).Format(time.RFC3339),
			"codex_7d_reset_at":                      now.Add(24 * time.Hour).Format(time.RFC3339),
		},
	}
	for key, value := range extra {
		account.Extra[key] = value
	}
	return account
}

func newAutoResetLowUsage(now time.Time, creditID string, expiresAt time.Time) *OpenAIQuotaUsage {
	expiry := expiresAt.Format(time.RFC3339)
	return &OpenAIQuotaUsage{
		FetchedAt: now.Unix(),
		RateLimit: &OpenAIRateLimit{
			PrimaryWindow:   &OpenAIRateLimitWindow{UsedPercent: 10, LimitWindowSeconds: 5 * 60 * 60, ResetAfterSeconds: 3600, ResetAt: now.Add(time.Hour).Unix()},
			SecondaryWindow: &OpenAIRateLimitWindow{UsedPercent: 10, LimitWindowSeconds: 7 * 24 * 60 * 60, ResetAfterSeconds: 86400, ResetAt: now.Add(24 * time.Hour).Unix()},
		},
		RateLimitResetCredits: &OpenAIRateLimitResetCredits{
			AvailableCount: 1,
			Credits:        []OpenAIRateLimitResetCreditDetail{{ExpiresAt: expiry}},
		},
		autoResetCandidates: []openAIAutoResetCreditCandidate{{ID: creditID, ExpiresAt: expiry}},
		upstreamTime:        now,
	}
}

func newAutoResetExpiringCardAccount(now time.Time, extra map[string]any) *Account {
	merged := map[string]any{
		OpenAIAutoResetCreditExpiryEnabledExtraKey:     true,
		OpenAIAutoResetCreditExpiryLeadMinutesExtraKey: 1440.0,
		openaiQuotaResetCreditsSyncedAtKey:             now.Format(time.RFC3339),
		openaiQuotaResetCreditsKey: map[string]any{
			"available_count": 1,
			"credits":         []any{map[string]any{"expires_at": now.Add(2 * time.Hour).Format(time.RFC3339)}},
		},
	}
	for key, value := range extra {
		merged[key] = value
	}
	return newAutoResetLowUsageAccount(now, merged)
}

func TestOpenAIQuotaAutoResetService_ExpiringCreditConsumedAfterLiveCheck(t *testing.T) {
	now := time.Now().UTC()
	cachedExpiry := now.Add(2 * time.Hour).Format(time.RFC3339)
	account := newAutoResetLowUsageAccount(now, map[string]any{
		OpenAIAutoResetCreditExpiryEnabledExtraKey:     true,
		OpenAIAutoResetCreditExpiryLeadMinutesExtraKey: 1440.0,
		openaiQuotaResetCreditsSyncedAtKey:             now.Format(time.RFC3339),
		openaiQuotaResetCreditsKey: map[string]any{
			"available_count": 1,
			"credits":         []any{map[string]any{"expires_at": cachedExpiry}},
		},
	})
	repo := &autoResetTestAccountRepo{account: account}
	quota := &autoResetTestQuota{usage: newAutoResetLowUsage(now, "credit-expiring", now.Add(2*time.Hour))}

	require.NoError(t, newAutoResetTestService(repo, quota).evaluateAccount(context.Background(), account.ID))

	require.Equal(t, int32(1), quota.resetCalls.Load())
	require.Equal(t, "credit-expiring", quota.resetArgs[0][0])
	state := openAIAutoResetStateFromExtra(repo.account.Extra)
	require.NotNil(t, state)
	require.Equal(t, OpenAIAutoResetStatusSuccess, state.Status)
	require.Equal(t, "expiry", state.TriggerWindow)
}

func TestOpenAIQuotaAutoResetService_ExpiringCreditGoneOnLiveCheckIsNotConsumed(t *testing.T) {
	now := time.Now().UTC()
	account := newAutoResetLowUsageAccount(now, map[string]any{
		OpenAIAutoResetCreditExpiryEnabledExtraKey:     true,
		OpenAIAutoResetCreditExpiryLeadMinutesExtraKey: 1440.0,
		openaiQuotaResetCreditsSyncedAtKey:             now.Format(time.RFC3339),
		openaiQuotaResetCreditsKey: map[string]any{
			"available_count": 1,
			"credits":         []any{map[string]any{"expires_at": now.Add(2 * time.Hour).Format(time.RFC3339)}},
		},
	})
	repo := &autoResetTestAccountRepo{account: account}
	quota := &autoResetTestQuota{usage: newAutoResetLowUsage(now, "credit-fresh", now.Add(10*24*time.Hour))}
	service := newAutoResetTestService(repo, quota)
	t.Cleanup(service.Stop)

	require.NoError(t, service.evaluateAccount(context.Background(), account.ID))

	require.Equal(t, int32(0), quota.resetCalls.Load())
	require.Equal(t, int32(1), quota.queryCalls.Load())
	state := openAIAutoResetStateFromExtra(repo.account.Extra)
	require.NotNil(t, state)
	require.Equal(t, OpenAIAutoResetStatusAvailable, state.Status)
	require.Equal(t, 1, state.AvailableCount)
	_, armed := service.expiryTimers.Load(account.ID)
	require.True(t, armed, "卡未进窗时按实时明细排定时器")
}

func TestOpenAIQuotaAutoResetService_RefreshesCreditSnapshotDaily(t *testing.T) {
	now := time.Now().UTC()
	account := newAutoResetLowUsageAccount(now, nil)
	repo := &autoResetTestAccountRepo{account: account}
	quota := &autoResetTestQuota{usage: newAutoResetLowUsage(now, "credit-id", now.Add(10*24*time.Hour))}
	service := newAutoResetTestService(repo, quota)

	require.NoError(t, service.evaluateAccount(context.Background(), account.ID))
	require.Equal(t, int32(1), quota.queryCalls.Load(), "进程内首次评估必须取一次卡信息")
	require.Equal(t, int32(1), quota.cacheCalls.Load())
	require.Equal(t, int32(0), quota.resetCalls.Load())

	require.NoError(t, service.evaluateAccount(context.Background(), account.ID))
	require.Equal(t, int32(1), quota.queryCalls.Load(), "24 小时内不重复取")

	service.fetchStates.Store(account.ID, openAIAutoResetFetchState{nextAt: time.Now().Add(-time.Second), fetched: true})
	require.NoError(t, service.evaluateAccount(context.Background(), account.ID))
	require.Equal(t, int32(2), quota.queryCalls.Load(), "到期后再取一次")

	service.fetchStates.Store(account.ID, openAIAutoResetFetchState{nextAt: time.Now().Add(time.Hour)})
	repo.mu.Lock()
	repo.account.Extra["codex_usage_updated_at"] = now.Add(-time.Hour).Format(time.RFC3339)
	repo.mu.Unlock()
	require.NoError(t, service.evaluateAccount(context.Background(), account.ID))
	require.Equal(t, int32(2), quota.queryCalls.Load(), "启动错峰未轮到前，用量快照过期也不提前实查")
}

// 用量查询失败（如代理不通）后，下一次扫描必须继续重查：到期用卡有截止时间，
// 每分钟重试是它的失败恢复路径，不能用长退避换日志安静。
func TestOpenAIQuotaAutoResetService_QueryFailureRetriesOnNextScan(t *testing.T) {
	now := time.Now().UTC()
	account := newAutoResetLowUsageAccount(now, map[string]any{
		"codex_usage_updated_at": now.Add(-time.Hour).Format(time.RFC3339),
	})
	repo := &autoResetTestAccountRepo{account: account}
	quota := &autoResetTestQuota{
		usage:    newAutoResetLowUsage(now, "credit-id", now.Add(10*24*time.Hour)),
		queryErr: errors.New("proxy down"),
	}
	service := newAutoResetTestService(repo, quota)
	service.refreshSchedulerLease(context.Background())
	require.True(t, service.isSchedulerLeader())
	service.fetchStates.Store(account.ID, openAIAutoResetFetchState{nextAt: now.Add(-time.Second), fetched: true})

	require.Error(t, service.evaluateAccount(context.Background(), account.ID))
	require.Equal(t, int32(1), quota.queryCalls.Load())
	state := openAIAutoResetStateFromExtra(repo.account.Extra)
	require.NotNil(t, state)
	require.Equal(t, OpenAIAutoResetStatusFailed, state.Status)
	require.Equal(t, "RESET_CREDIT_QUERY_FAILED", state.ErrorCode)

	quota.queryErr = nil
	require.NoError(t, service.evaluateAccount(context.Background(), account.ID))
	require.Equal(t, int32(2), quota.queryCalls.Load(), "上游恢复后的下一次扫描应立即重查")
	state = openAIAutoResetStateFromExtra(repo.account.Extra)
	require.NotNil(t, state)
	require.NotEqual(t, OpenAIAutoResetStatusFailed, state.Status, "重查成功后应脱离失败态")
}

func TestOpenAIQuotaAutoResetService_IncompleteCreditDetailsReportPreciseCode(t *testing.T) {
	now := time.Now().UTC()
	account := newAutoResetLowUsageAccount(now, nil)
	repo := &autoResetTestAccountRepo{account: account}
	quota := &autoResetTestQuota{
		usage:    newAutoResetLowUsage(now, "credit-id", now.Add(10*24*time.Hour)),
		cacheErr: errOpenAIQuotaResetCreditsRefreshFailed,
	}

	service := newAutoResetTestService(repo, quota)
	_ = service.evaluateAccount(context.Background(), account.ID)

	state := openAIAutoResetStateFromExtra(repo.account.Extra)
	require.NotNil(t, state)
	require.Equal(t, OpenAIAutoResetStatusFailed, state.Status)
	require.Equal(t, "RESET_CREDIT_DETAILS_INCOMPLETE", state.ErrorCode)

	_ = service.evaluateAccount(context.Background(), account.ID)
	require.Equal(t, int32(1), quota.queryCalls.Load(), "已取过一次，24 小时内不因明细缺失重取")
}

func TestOpenAIQuotaAutoResetService_ExpiryUsesUpstreamClock(t *testing.T) {
	now := time.Now().UTC()
	account := newAutoResetExpiringCardAccount(now, nil)
	repo := &autoResetTestAccountRepo{account: account}
	usage := newAutoResetLowUsage(now, "credit-expiring", now.Add(2*time.Hour))
	usage.upstreamTime = now.Add(-30 * time.Hour)
	quota := &autoResetTestQuota{usage: usage}

	service := newAutoResetTestService(repo, quota)
	t.Cleanup(service.Stop)

	require.NoError(t, service.evaluateAccount(context.Background(), account.ID))

	require.Equal(t, int32(0), quota.resetCalls.Load(), "按上游时间该卡还剩 32 小时，不在提前窗内")
	state := openAIAutoResetStateFromExtra(repo.account.Extra)
	require.NotNil(t, state)
	require.Equal(t, OpenAIAutoResetStatusAvailable, state.Status)
	_, armed := service.expiryTimers.Load(account.ID)
	require.True(t, armed)
}

func TestOpenAIQuotaAutoResetService_LocalClockDoesNotAffectExpiry(t *testing.T) {
	now := time.Now().UTC()
	account := newAutoResetExpiringCardAccount(now, nil)
	repo := &autoResetTestAccountRepo{account: account}
	usage := newAutoResetLowUsage(now, "credit-expiring", now.Add(2*time.Hour))
	usage.upstreamTime = now.Add(10 * time.Minute)
	quota := &autoResetTestQuota{usage: usage}

	require.NoError(t, newAutoResetTestService(repo, quota).evaluateAccount(context.Background(), account.ID))

	require.Equal(t, int32(1), quota.resetCalls.Load(), "本机时钟偏差只记日志，不影响按上游时间的判定")
	state := openAIAutoResetStateFromExtra(repo.account.Extra)
	require.NotNil(t, state)
	require.Equal(t, OpenAIAutoResetStatusSuccess, state.Status)
}

func TestOpenAIQuotaAutoResetService_UpstreamTimeMissingDoesNotConsume(t *testing.T) {
	now := time.Now().UTC()
	account := newAutoResetExpiringCardAccount(now, nil)
	repo := &autoResetTestAccountRepo{account: account}
	usage := newAutoResetLowUsage(now, "credit-expiring", now.Add(2*time.Hour))
	usage.upstreamTime = time.Time{}
	quota := &autoResetTestQuota{usage: usage}
	service := newAutoResetTestService(repo, quota)
	t.Cleanup(service.Stop)

	require.NoError(t, service.evaluateAccount(context.Background(), account.ID))

	require.Equal(t, int32(0), quota.resetCalls.Load())
	state := openAIAutoResetStateFromExtra(repo.account.Extra)
	require.NotNil(t, state)
	require.Equal(t, OpenAIAutoResetStatusAvailable, state.Status)
	_, armed := service.expiryTimers.Load(account.ID)
	require.False(t, armed, "没有上游时间不排定时器")
	fetch, ok := service.fetchStates.Load(account.ID)
	require.True(t, ok)
	fetchState, _ := fetch.(openAIAutoResetFetchState)
	require.LessOrEqual(t, time.Until(fetchState.nextAt), openAIAutoResetSnapshotTTL, "拿不到上游时间时按 10 分钟节奏继续取")

	usage.upstreamTime = now
	service.fetchStates.Store(account.ID, openAIAutoResetFetchState{nextAt: time.Now().Add(-time.Second), fetched: true})
	require.NoError(t, service.evaluateAccount(context.Background(), account.ID))
	require.Equal(t, int32(1), quota.resetCalls.Load(), "拿到上游时间后按实时明细判定并用卡")
}

func TestOpenAIQuotaAutoResetService_ExpiryTimerTriggersFinalCheck(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	account := newAutoResetLowUsageAccount(now, map[string]any{OpenAIAutoResetCreditExpiryEnabledExtraKey: true, OpenAIAutoResetCreditExpiryLeadMinutesExtraKey: 1440.0})
	repo := &autoResetTestAccountRepo{account: account}
	expiresAt := now.Add(24*time.Hour + time.Second)
	quota := &autoResetTestQuota{usage: newAutoResetLowUsage(now, "credit-expiring", expiresAt)}
	service := newAutoResetTestService(repo, quota)
	t.Cleanup(service.Stop)

	require.NoError(t, service.evaluateAccount(context.Background(), account.ID))
	require.Equal(t, int32(0), quota.resetCalls.Load(), "还差 1 秒进窗，只排定时器")
	_, armed := service.expiryTimers.Load(account.ID)
	require.True(t, armed)

	require.Eventually(t, func() bool {
		_, due := service.expiryDue.Load(account.ID)
		return due
	}, 3*time.Second, 10*time.Millisecond, "定时器到点应打上到期待核标记")

	later := newAutoResetLowUsage(now, "credit-expiring", expiresAt)
	later.upstreamTime = now.Add(2 * time.Second)
	quota.usage = later
	require.NoError(t, service.evaluateAccount(context.Background(), account.ID))
	require.Equal(t, int32(1), quota.resetCalls.Load(), "最终校验通过后用卡")
	require.Equal(t, "credit-expiring", quota.resetArgs[0][0])
}

func TestOpenAIQuotaAutoResetService_DisablingDisarmsExpiryTimer(t *testing.T) {
	now := time.Now().UTC()
	account := newAutoResetLowUsageAccount(now, map[string]any{OpenAIAutoResetCreditExpiryEnabledExtraKey: true, OpenAIAutoResetCreditExpiryLeadMinutesExtraKey: 1440.0})
	repo := &autoResetTestAccountRepo{account: account}
	quota := &autoResetTestQuota{usage: newAutoResetLowUsage(now, "credit-id", now.Add(10*24*time.Hour))}
	service := newAutoResetTestService(repo, quota)
	t.Cleanup(service.Stop)

	require.NoError(t, service.evaluateAccount(context.Background(), account.ID))
	_, armed := service.expiryTimers.Load(account.ID)
	require.True(t, armed)
	require.NotNil(t, repo.account.Extra[OpenAIAutoResetCreditExpiryAtExtraKey])

	repo.mu.Lock()
	repo.account.Extra[OpenAIAutoResetCreditExpiryEnabledExtraKey] = false
	repo.mu.Unlock()
	require.NoError(t, service.evaluateAccount(context.Background(), account.ID))
	_, armed = service.expiryTimers.Load(account.ID)
	require.False(t, armed, "关闭到期开关后撤销定时器，即使阈值用卡仍开着")
	require.Nil(t, repo.account.Extra[OpenAIAutoResetCreditExpiryAtExtraKey], "撤销定时器后清空计划触发时刻")
}

type autoResetTestListRepo struct {
	AccountRepository
	accounts []Account
}

func (r *autoResetTestListRepo) ListWithFilters(context.Context, pagination.PaginationParams, string, string, string, string, int64, string) ([]Account, *pagination.PaginationResult, error) {
	return r.accounts, &pagination.PaginationResult{Pages: 1}, nil
}

func TestOpenAIQuotaAutoResetService_StaggersInitialFetch(t *testing.T) {
	previous := openAIAutoResetInitialFetchInterval
	openAIAutoResetInitialFetchInterval = 10 * time.Millisecond
	t.Cleanup(func() { openAIAutoResetInitialFetchInterval = previous })

	mk := func(id int64, schedulable bool, extra map[string]any) Account {
		return Account{ID: id, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Schedulable: schedulable, Extra: extra}
	}
	thresholdOn := map[string]any{OpenAIAutoResetCreditEnabledExtraKey: true}
	expiryOn := map[string]any{OpenAIAutoResetCreditExpiryEnabledExtraKey: true}
	repo := &autoResetTestListRepo{accounts: []Account{mk(1, true, thresholdOn), mk(2, true, thresholdOn), mk(3, true, expiryOn), mk(4, false, thresholdOn), mk(5, true, nil)}}
	service := NewOpenAIQuotaAutoResetService(repo, &autoResetTestQuota{}, autoResetTestRecoverer{}, nil, nil, nil, nil)
	t.Cleanup(service.Stop)

	service.scheduleInitialFetch(context.Background())

	var order []int64
	for len(order) < 3 {
		select {
		case id := <-service.queue:
			order = append(order, id)
		case <-time.After(time.Second):
			t.Fatalf("只收到 %d 个启动取卡通知", len(order))
		}
	}
	require.ElementsMatch(t, []int64{1, 2, 3}, order, "阈值或到期任一开启的账号参与启动错峰")
	for i := 1; i < len(order); i++ {
		prev, _ := service.fetchStates.Load(order[i-1])
		next, _ := service.fetchStates.Load(order[i])
		prevState, _ := prev.(openAIAutoResetFetchState)
		nextState, _ := next.(openAIAutoResetFetchState)
		require.False(t, nextState.nextAt.Before(prevState.nextAt), "通知顺序应与错峰计划一致")
	}
	select {
	case id := <-service.queue:
		t.Fatalf("未开启或不可调度的账号 %d 不应被取卡", id)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestOpenAIQuotaAutoResetService_SchedulerLeaseGatesScheduledFetch(t *testing.T) {
	now := time.Now().UTC()
	lock := &fakeLeaderLockCache{}
	newInstance := func(quota *autoResetTestQuota) (*OpenAIQuotaAutoResetService, *autoResetTestAccountRepo) {
		repo := &autoResetTestAccountRepo{account: newAutoResetLowUsageAccount(now, map[string]any{OpenAIAutoResetCreditExpiryEnabledExtraKey: true, OpenAIAutoResetCreditExpiryLeadMinutesExtraKey: 1440.0})}
		config := DefaultIdempotencyConfig()
		config.ObserveOnly = false
		service := NewOpenAIQuotaAutoResetService(repo, quota, autoResetTestRecoverer{}, NewIdempotencyCoordinator(newInMemoryIdempotencyRepo(), config), nil, nil, lock)
		t.Cleanup(service.Stop)
		return service, repo
	}
	quotaA := &autoResetTestQuota{usage: newAutoResetLowUsage(now, "credit-a", now.Add(10*24*time.Hour))}
	quotaB := &autoResetTestQuota{usage: newAutoResetLowUsage(now, "credit-b", now.Add(10*24*time.Hour))}
	leader, _ := newInstance(quotaA)
	follower, followerRepo := newInstance(quotaB)

	leader.refreshSchedulerLease(context.Background())
	follower.refreshSchedulerLease(context.Background())
	require.True(t, leader.isSchedulerLeader())
	require.False(t, follower.isSchedulerLeader())

	require.NoError(t, follower.evaluateAccount(context.Background(), 7))
	require.Equal(t, int32(0), quotaB.queryCalls.Load(), "非领导实例不做计划内取卡")
	_, armed := follower.expiryTimers.Load(int64(7))
	require.False(t, armed)

	require.NoError(t, leader.evaluateAccount(context.Background(), 7))
	require.Equal(t, int32(1), quotaA.queryCalls.Load())
	_, armed = leader.expiryTimers.Load(int64(7))
	require.True(t, armed)

	followerRepo.mu.Lock()
	followerRepo.account.Extra["codex_5h_used_percent"] = 100.0
	followerRepo.mu.Unlock()
	_ = follower.evaluateAccount(context.Background(), 7)
	require.Equal(t, int32(1), quotaB.queryCalls.Load(), "用量阈值触发不受领导租约限制")
	_, armed = follower.expiryTimers.Load(int64(7))
	require.False(t, armed, "非领导实例即使实查过也不设定时器")

	lock.mu.Lock()
	lock.owners[openAIAutoResetSchedulerLockKey] = "someone-else"
	lock.mu.Unlock()
	leader.refreshSchedulerLease(context.Background())
	require.False(t, leader.isSchedulerLeader())
	_, armed = leader.expiryTimers.Load(int64(7))
	require.False(t, armed, "失去租约后撤销全部定时器")

	lock.mu.Lock()
	delete(lock.owners, openAIAutoResetSchedulerLockKey)
	lock.mu.Unlock()
	follower.refreshSchedulerLease(context.Background())
	require.True(t, follower.isSchedulerLeader(), "租约空出后另一实例接任")
	_, scheduled := follower.fetchStates.Load(int64(7))
	require.True(t, scheduled, "接任时重建错峰取卡计划")
}

func TestOpenAIQuotaAutoResetService_WithoutLockEveryInstanceSchedules(t *testing.T) {
	service := NewOpenAIQuotaAutoResetService(&autoResetTestAccountRepo{account: &Account{}}, &autoResetTestQuota{}, autoResetTestRecoverer{}, nil, nil, nil, nil)
	t.Cleanup(service.Stop)
	require.True(t, service.isSchedulerLeader(), "没有 Redis 时退化为单实例")
}

func TestOpenAIQuotaAutoResetService_ExpiryOnlyUsesCardAndIgnoresThreshold(t *testing.T) {
	now := time.Now().UTC()
	account := newAutoResetExpiringCardAccount(now, map[string]any{
		OpenAIAutoResetCreditEnabledExtraKey: false,
		"codex_5h_used_percent":              100.0,
	})
	repo := &autoResetTestAccountRepo{account: account}
	usage := newAutoResetLowUsage(now, "credit-expiring", now.Add(2*time.Hour))
	usage.RateLimit.PrimaryWindow.UsedPercent = 100
	quota := &autoResetTestQuota{usage: usage}
	service := newAutoResetTestService(repo, quota)
	t.Cleanup(service.Stop)

	require.NoError(t, service.evaluateAccount(context.Background(), account.ID))

	require.Equal(t, int32(1), quota.resetCalls.Load(), "只开到期开关也能按到期用卡")
	state := openAIAutoResetStateFromExtra(repo.account.Extra)
	require.NotNil(t, state)
	require.Equal(t, "expiry", state.TriggerWindow, "阈值开关关闭时 100% 用量不参与触发")
}

func TestOpenAIQuotaAutoResetService_ExpiryOnlyWithoutExpiringCardDoesNotConsume(t *testing.T) {
	now := time.Now().UTC()
	account := newAutoResetLowUsageAccount(now, map[string]any{
		OpenAIAutoResetCreditExpiryEnabledExtraKey:     true,
		OpenAIAutoResetCreditExpiryLeadMinutesExtraKey: 1440.0,
		OpenAIAutoResetCreditEnabledExtraKey:           false,
		"codex_5h_used_percent":                        100.0,
	})
	repo := &autoResetTestAccountRepo{account: account}
	usage := newAutoResetLowUsage(now, "credit-far", now.Add(10*24*time.Hour))
	usage.RateLimit.PrimaryWindow.UsedPercent = 100
	quota := &autoResetTestQuota{usage: usage}
	service := newAutoResetTestService(repo, quota)
	t.Cleanup(service.Stop)

	require.NoError(t, service.evaluateAccount(context.Background(), account.ID))

	require.Equal(t, int32(0), quota.resetCalls.Load(), "阈值开关关闭时即使 100% 也不用卡")
	_, armed := service.expiryTimers.Load(account.ID)
	require.True(t, armed, "到期开关开启时按最早到期的卡排定时器")
	wantFireAt := now.Add(10 * 24 * time.Hour).Truncate(time.Second).Add(-24 * time.Hour).UTC().Format(time.RFC3339)
	require.Equal(t, wantFireAt, repo.account.Extra[OpenAIAutoResetCreditExpiryAtExtraKey], "计划触发时刻写入 extra 供列表显示")
}

func TestOpenAIQuotaAutoResetService_ConfigChangeTriggersRefetch(t *testing.T) {
	now := time.Now().UTC()
	account := newAutoResetLowUsageAccount(now, nil)
	repo := &autoResetTestAccountRepo{account: account}
	quota := &autoResetTestQuota{usage: newAutoResetLowUsage(now, "credit-id", now.Add(10*24*time.Hour))}
	service := newAutoResetTestService(repo, quota)
	t.Cleanup(service.Stop)

	require.NoError(t, service.evaluateAccount(context.Background(), account.ID))
	require.Equal(t, int32(1), quota.queryCalls.Load())
	_, armed := service.expiryTimers.Load(account.ID)
	require.False(t, armed, "只开阈值用卡时没有定时器")

	repo.mu.Lock()
	repo.account.Extra[OpenAIAutoResetCreditExpiryEnabledExtraKey] = true
	repo.account.Extra[OpenAIAutoResetCreditExpiryLeadMinutesExtraKey] = 1440.0
	repo.mu.Unlock()
	require.NoError(t, service.evaluateAccount(context.Background(), account.ID))
	require.Equal(t, int32(2), quota.queryCalls.Load(), "开启到期用卡后立即重取并排定时器")
	_, armed = service.expiryTimers.Load(account.ID)
	require.True(t, armed)

	require.NoError(t, service.evaluateAccount(context.Background(), account.ID))
	require.Equal(t, int32(2), quota.queryCalls.Load(), "配置不变时 24 小时内不重复取")

	repo.mu.Lock()
	repo.account.Extra[OpenAIAutoResetCreditEnabledExtraKey] = false
	repo.account.Extra[OpenAIAutoResetCreditExpiryEnabledExtraKey] = false
	repo.mu.Unlock()
	require.NoError(t, service.evaluateAccount(context.Background(), account.ID))
	require.Equal(t, int32(2), quota.queryCalls.Load(), "全部关闭后不再取信息")
	_, armed = service.expiryTimers.Load(account.ID)
	require.False(t, armed, "全部关闭后撤销定时器")
}

func TestOpenAIQuotaAutoResetService_ManualCreditSyncRearmsExpiryTimer(t *testing.T) {
	now := time.Now().UTC()
	account := newAutoResetLowUsageAccount(now, map[string]any{
		OpenAIAutoResetCreditEnabledExtraKey:           false,
		OpenAIAutoResetCreditExpiryEnabledExtraKey:     true,
		OpenAIAutoResetCreditExpiryLeadMinutesExtraKey: 1440.0,
	})
	repo := &autoResetTestAccountRepo{account: account}
	quota := &autoResetTestQuota{usage: newAutoResetLowUsage(now, "credit-old", now.Add(10*24*time.Hour))}
	service := newAutoResetTestService(repo, quota)
	t.Cleanup(service.Stop)

	require.NoError(t, service.evaluateAccount(context.Background(), account.ID))
	_, armed := service.expiryTimers.Load(account.ID)
	require.True(t, armed)

	consumed := newAutoResetLowUsage(now, "", now)
	consumed.RateLimitResetCredits = &OpenAIRateLimitResetCredits{}
	consumed.autoResetCandidates = nil
	service.syncCreditUsage(context.Background(), account.ID, consumed)
	_, armed = service.expiryTimers.Load(account.ID)
	require.False(t, armed, "手动用掉唯一一张卡后撤销定时器")
	require.Nil(t, repo.account.Extra[OpenAIAutoResetCreditExpiryAtExtraKey], "撤销定时器后清空计划触发时刻")
	state := openAIAutoResetStateFromExtra(repo.account.Extra)
	require.NotNil(t, state)
	require.Equal(t, OpenAIAutoResetStatusAvailable, state.Status, "运行态只由调度器实查后写入，手动同步不碰")
	require.Equal(t, 1, state.AvailableCount)

	regranted := newAutoResetLowUsage(now, "credit-new", now.Add(3*24*time.Hour))
	service.syncCreditUsage(context.Background(), account.ID, regranted)
	_, armed = service.expiryTimers.Load(account.ID)
	require.True(t, armed, "手动查询到新卡后按新卡排定时器")
	wantFireAt := now.Add(3 * 24 * time.Hour).Truncate(time.Second).Add(-24 * time.Hour).UTC().Format(time.RFC3339)
	require.Equal(t, wantFireAt, repo.account.Extra[OpenAIAutoResetCreditExpiryAtExtraKey])
	require.Equal(t, int32(1), quota.queryCalls.Load(), "同步只用手动路径已取到的结果，不额外实查")
}

func TestOpenAIQuotaAutoResetService_ManualCreditSyncReplacesScheduledFetch(t *testing.T) {
	now := time.Now().UTC()
	account := newAutoResetLowUsageAccount(now, map[string]any{OpenAIAutoResetCreditExpiryEnabledExtraKey: true, OpenAIAutoResetCreditExpiryLeadMinutesExtraKey: 1440.0})
	repo := &autoResetTestAccountRepo{account: account}
	quota := &autoResetTestQuota{usage: newAutoResetLowUsage(now, "credit-id", now.Add(10*24*time.Hour))}
	service := newAutoResetTestService(repo, quota)
	t.Cleanup(service.Stop)

	service.syncCreditUsage(context.Background(), account.ID, newAutoResetLowUsage(now, "credit-id", now.Add(10*24*time.Hour)))
	_, armed := service.expiryTimers.Load(account.ID)
	require.True(t, armed)

	require.NoError(t, service.evaluateAccount(context.Background(), account.ID))
	require.Equal(t, int32(0), quota.queryCalls.Load(), "手动路径的实查结果顶替本轮计划取卡")
}

func TestOpenAIQuotaAutoResetService_ManualCreditSyncKeepsInFlightAttempt(t *testing.T) {
	now := time.Now().UTC()
	account := newAutoResetLowUsageAccount(now, map[string]any{
		OpenAIAutoResetCreditExpiryEnabledExtraKey:     true,
		OpenAIAutoResetCreditExpiryLeadMinutesExtraKey: 1440.0,
		OpenAIAutoResetCreditStateExtraKey: OpenAIAutoResetCreditState{
			Status: OpenAIAutoResetStatusResetting, AvailableCount: 1, AttemptCycleHash: "cycle", AttemptCreditHash: "credit",
		},
	})
	repo := &autoResetTestAccountRepo{account: account}
	service := newAutoResetTestService(repo, &autoResetTestQuota{})
	t.Cleanup(service.Stop)

	service.syncCreditUsage(context.Background(), account.ID, newAutoResetLowUsage(now, "credit-id", now.Add(10*24*time.Hour)))

	state := openAIAutoResetStateFromExtra(repo.account.Extra)
	require.Equal(t, OpenAIAutoResetStatusResetting, state.Status, "自动用卡进行中不覆盖尝试指纹")
	require.Equal(t, "credit", state.AttemptCreditHash)
	_, armed := service.expiryTimers.Load(account.ID)
	require.True(t, armed, "定时器仍按最新卡信息重排")
}

func TestOpenAIQuotaAutoResetService_ManualCreditSyncOnFollowerLeavesTimerToLeader(t *testing.T) {
	now := time.Now().UTC()
	lock := &fakeLeaderLockCache{owners: map[string]string{openAIAutoResetSchedulerLockKey: "someone-else"}}
	account := newAutoResetLowUsageAccount(now, map[string]any{
		OpenAIAutoResetCreditExpiryEnabledExtraKey:     true,
		OpenAIAutoResetCreditExpiryLeadMinutesExtraKey: 1440.0,
		OpenAIAutoResetCreditExpiryAtExtraKey:          "2099-01-01T00:00:00Z",
	})
	repo := &autoResetTestAccountRepo{account: account}
	config := DefaultIdempotencyConfig()
	config.ObserveOnly = false
	service := NewOpenAIQuotaAutoResetService(repo, &autoResetTestQuota{}, autoResetTestRecoverer{}, NewIdempotencyCoordinator(newInMemoryIdempotencyRepo(), config), nil, nil, lock)
	t.Cleanup(service.Stop)
	service.refreshSchedulerLease(context.Background())
	require.False(t, service.isSchedulerLeader())

	service.syncCreditUsage(context.Background(), account.ID, newAutoResetLowUsage(now, "credit-id", now.Add(10*24*time.Hour)))

	_, armed := service.expiryTimers.Load(account.ID)
	require.False(t, armed, "非领导实例不设定时器")
	require.Equal(t, "2099-01-01T00:00:00Z", repo.account.Extra[OpenAIAutoResetCreditExpiryAtExtraKey], "计划时刻留给领导实例重排")
	require.Nil(t, repo.account.Extra[OpenAIAutoResetCreditStateExtraKey], "手动同步不写运行态")
}

func TestSyncOpenAIAutoResetCredit_DelegatesToRegisteredService(t *testing.T) {
	now := time.Now().UTC()
	account := newAutoResetLowUsageAccount(now, map[string]any{OpenAIAutoResetCreditExpiryEnabledExtraKey: true, OpenAIAutoResetCreditExpiryLeadMinutesExtraKey: 1440.0})
	repo := &autoResetTestAccountRepo{account: account}
	service := newAutoResetTestService(repo, &autoResetTestQuota{})
	t.Cleanup(service.Stop)
	usage := newAutoResetLowUsage(now, "credit-id", now.Add(10*24*time.Hour))

	SyncOpenAIAutoResetCredit(context.Background(), account.ID, usage)
	_, armed := service.expiryTimers.Load(account.ID)
	require.False(t, armed, "未注册调度器时静默忽略")

	setOpenAIAutoResetNotifier(service)
	t.Cleanup(func() { clearOpenAIAutoResetNotifier(service) })
	SyncOpenAIAutoResetCredit(context.Background(), account.ID, usage)
	_, armed = service.expiryTimers.Load(account.ID)
	require.True(t, armed)
}

func TestOpenAIQuotaAutoResetService_ManualCreditSyncKeepsFailedAttemptFingerprint(t *testing.T) {
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
	usage := &OpenAIQuotaUsage{
		FetchedAt: now.Unix(),
		RateLimit: &OpenAIRateLimit{
			PrimaryWindow: &OpenAIRateLimitWindow{UsedPercent: 100, LimitWindowSeconds: 5 * 60 * 60, ResetAfterSeconds: 3600, ResetAt: now.Add(time.Hour).Unix()},
		},
		RateLimitResetCredits: &OpenAIRateLimitResetCredits{
			AvailableCount: 1,
			Credits:        []OpenAIRateLimitResetCreditDetail{{ExpiresAt: expiresAt}},
		},
		autoResetCandidates: []openAIAutoResetCreditCandidate{{ID: "credit-a", ExpiresAt: expiresAt}},
		upstreamTime:        now,
	}
	quota := &autoResetTestQuota{failFirst: true, usage: usage}
	idempotencyConfig := DefaultIdempotencyConfig()
	idempotencyConfig.ObserveOnly = false
	idempotencyConfig.FailedRetryBackoff = 0
	service := NewOpenAIQuotaAutoResetService(repo, quota, autoResetTestRecoverer{}, NewIdempotencyCoordinator(newInMemoryIdempotencyRepo(), idempotencyConfig), nil, nil, nil)
	t.Cleanup(service.Stop)

	require.Error(t, service.evaluateAccount(context.Background(), account.ID), "首次消费超时，结果未明")
	state := openAIAutoResetStateFromExtra(repo.account.Extra)
	require.Equal(t, OpenAIAutoResetStatusFailed, state.Status)
	require.NotEmpty(t, state.AttemptCreditHash)

	replaced := *usage
	replaced.autoResetCandidates = []openAIAutoResetCreditCandidate{{ID: "credit-b", ExpiresAt: expiresAt}}
	service.syncCreditUsage(context.Background(), account.ID, &replaced)
	state = openAIAutoResetStateFromExtra(repo.account.Extra)
	require.NotEmpty(t, state.AttemptCreditHash, "管理员刷新不得抹掉未明结果的尝试指纹")

	quota.usage = &replaced
	require.Error(t, service.evaluateAccount(context.Background(), account.ID))
	require.Equal(t, int32(1), quota.resetCalls.Load(), "原卡从明细消失时拒绝改选另一张卡")
}

func TestOpenAIQuotaAutoResetService_ManualCreditSyncTriggersFinalCheckForExpiringCard(t *testing.T) {
	now := time.Now().UTC()
	account := newAutoResetLowUsageAccount(now, map[string]any{
		OpenAIAutoResetCreditEnabledExtraKey:           false,
		OpenAIAutoResetCreditExpiryEnabledExtraKey:     true,
		OpenAIAutoResetCreditExpiryLeadMinutesExtraKey: 1440.0,
	})
	repo := &autoResetTestAccountRepo{account: account}
	usage := newAutoResetLowUsage(now, "credit-soon", now.Add(2*time.Hour))
	quota := &autoResetTestQuota{usage: usage}
	service := newAutoResetTestService(repo, quota)
	t.Cleanup(service.Stop)

	service.syncCreditUsage(context.Background(), account.ID, usage)
	_, armed := service.expiryTimers.Load(account.ID)
	require.False(t, armed, "已进入提前窗口的卡不设定时器")

	require.NoError(t, service.evaluateAccount(context.Background(), account.ID))
	require.Equal(t, int32(2), quota.queryCalls.Load(), "手动刷新发现卡已进入提前窗口时立即实查做最终校验，用卡后再刷新一次额度")
	require.Equal(t, int32(1), quota.resetCalls.Load(), "校验通过后用卡")
	require.Equal(t, "credit-soon", quota.resetArgs[0][0])
}

func TestOpenAIQuotaAutoResetService_ManualCreditSyncCorrectsPauseDecision(t *testing.T) {
	now := time.Now().UTC()
	newPauseZoneAccount := func(state OpenAIAutoResetCreditState) *Account {
		return newAutoResetLowUsageAccount(now, map[string]any{
			"auto_pause_5h_threshold":          0.8,
			"auto_pause_7d_disabled":           true,
			"codex_5h_used_percent":            90.0,
			OpenAIAutoResetCreditStateExtraKey: state,
		})
	}
	usageWithCards := func(count int) *OpenAIQuotaUsage {
		usage := newAutoResetLowUsage(now, "credit-id", now.Add(10*24*time.Hour))
		usage.RateLimit.PrimaryWindow.UsedPercent = 90
		if count == 0 {
			usage.RateLimitResetCredits = &OpenAIRateLimitResetCredits{}
			usage.autoResetCandidates = nil
		}
		return usage
	}
	freshState := func(status string, count int) OpenAIAutoResetCreditState {
		return OpenAIAutoResetCreditState{Status: status, AvailableCount: count, CheckedAt: now.Format(time.RFC3339)}
	}

	t.Run("旧状态有卡而实查无卡时立即校正并暂停", func(t *testing.T) {
		account := newPauseZoneAccount(freshState(OpenAIAutoResetStatusAvailable, 1))
		repo := &autoResetTestAccountRepo{account: account}
		quota := &autoResetTestQuota{usage: usageWithCards(0)}
		service := newAutoResetTestService(repo, quota)
		t.Cleanup(service.Stop)
		paused, _ := shouldAutoPauseOpenAIAccountByQuota(context.Background(), account)
		require.False(t, paused, "前提：旧运行态有卡，网关放行到用卡阈值")

		service.syncCreditUsage(context.Background(), account.ID, usageWithCards(0))
		require.NoError(t, service.evaluateAccount(context.Background(), account.ID))

		require.Equal(t, int32(1), quota.queryCalls.Load(), "卡数与运行态不一致时立即实查校正")
		refreshed, err := repo.GetByID(context.Background(), account.ID)
		require.NoError(t, err)
		paused, _ = shouldAutoPauseOpenAIAccountByQuota(context.Background(), refreshed)
		require.True(t, paused, "校正后无卡，按普通暂停阈值退出调度")
	})

	t.Run("旧状态无卡而实查有卡时立即校正并放行", func(t *testing.T) {
		account := newPauseZoneAccount(freshState(OpenAIAutoResetStatusNoCredit, 0))
		repo := &autoResetTestAccountRepo{account: account}
		quota := &autoResetTestQuota{usage: usageWithCards(1)}
		service := newAutoResetTestService(repo, quota)
		t.Cleanup(service.Stop)
		paused, _ := shouldAutoPauseOpenAIAccountByQuota(context.Background(), account)
		require.True(t, paused, "前提：旧运行态无卡，网关按普通阈值暂停")

		service.syncCreditUsage(context.Background(), account.ID, usageWithCards(1))
		require.NoError(t, service.evaluateAccount(context.Background(), account.ID))

		require.Equal(t, int32(1), quota.queryCalls.Load())
		refreshed, err := repo.GetByID(context.Background(), account.ID)
		require.NoError(t, err)
		paused, _ = shouldAutoPauseOpenAIAccountByQuota(context.Background(), refreshed)
		require.False(t, paused, "校正后有卡，放行到用卡阈值")
	})

	t.Run("卡数一致时不额外实查", func(t *testing.T) {
		account := newPauseZoneAccount(freshState(OpenAIAutoResetStatusAvailable, 1))
		repo := &autoResetTestAccountRepo{account: account}
		quota := &autoResetTestQuota{usage: usageWithCards(1)}
		service := newAutoResetTestService(repo, quota)
		t.Cleanup(service.Stop)

		service.syncCreditUsage(context.Background(), account.ID, usageWithCards(1))
		require.NoError(t, service.evaluateAccount(context.Background(), account.ID))

		require.Equal(t, int32(0), quota.queryCalls.Load(), "运行态已与实查一致，沿用手动路径的结果")
	})
}
