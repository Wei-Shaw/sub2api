//go:build unit

package service

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// 上游 7d_oi 窗口耗尽会给账号写入 model_rate_limits["claude-fable-5"]，冷却到窗口重置时间。
// 如果期间用户用重置卡让上游额度提前归零，本地原本没有任何撤销路径：请求一直在本地
// 被判为 model_rate_limited，直到旧的 rate_limit_reset_at 到期为止。
//
// 下面锁定恢复规则：只有上游**明确**回报 7d_oi 用量回落到耗尽线以下、且已有记录来自
// 窗口耗尽时，才按单 key 撤销；缺失数据、积分不足、窗口仍被拒、以及其它模型都不受影响。

type fableRecoveryRepo struct {
	sessionWindowMockRepo
	clearedScopes []string
	clearErr      error
}

func (r *fableRecoveryRepo) ClearModelRateLimit(_ context.Context, _ int64, scope string) error {
	r.clearedScopes = append(r.clearedScopes, scope)
	return r.clearErr
}

func newFableRecoveryAccount(reason string, extraLimits map[string]any) *Account {
	limits := map[string]any{
		anthropicFableRateLimitKey: map[string]any{
			"rate_limited_at":     time.Now().Add(-2 * time.Hour).UTC().Format(time.RFC3339),
			"rate_limit_reset_at": time.Now().Add(3 * 24 * time.Hour).UTC().Format(time.RFC3339),
			"reason":              reason,
		},
	}
	for scope, payload := range extraLimits {
		limits[scope] = payload
	}
	return &Account{
		ID:       42,
		Platform: PlatformAnthropic,
		Type:     AccountTypeOAuth,
		Extra:    map[string]any{modelRateLimitsKey: limits},
	}
}

func fableUsageHeaders(utilization string, rejected bool) http.Header {
	headers := http.Header{}
	headers.Set("anthropic-ratelimit-unified-5h-status", "allowed")
	if rejected {
		headers.Set("anthropic-ratelimit-unified-7d_oi-status", "rejected")
	}
	if utilization != "" {
		headers.Set("anthropic-ratelimit-unified-7d_oi-utilization", utilization)
	}
	headers.Set("anthropic-ratelimit-unified-7d_oi-reset", strconv.FormatInt(time.Now().Add(7*24*time.Hour).Unix(), 10))
	return headers
}

func TestPassiveUsageClearsExhaustedFableLimitAfterQuotaReset(t *testing.T) {
	ctx := context.Background()
	repo := &fableRecoveryRepo{}
	svc := &RateLimitService{accountRepo: repo}
	account := newFableRecoveryAccount(anthropicFableWindowReason, nil)

	require.True(t, account.isModelRateLimitedWithContext(ctx, "claude-fable-5-2"),
		"前提：窗口耗尽后 Fable 请求被本地拦截")

	// 重置卡生效：上游回报该窗口 0%。
	svc.samplePassiveUsageFromHeaders(ctx, account, fableUsageHeaders("0", false))

	require.Equal(t, []string{anthropicFableRateLimitKey}, repo.clearedScopes,
		"必须只按 claude-fable-5 这一个 scope 撤销")
	require.False(t, account.isModelRateLimitedWithContext(ctx, "claude-fable-5-2"),
		"本实例的账号快照要同步解除，否则当前实例仍会继续跳过该账号")
}

func TestPassiveUsageKeepsFableLimitWhenUtilizationMissing(t *testing.T) {
	ctx := context.Background()
	repo := &fableRecoveryRepo{}
	svc := &RateLimitService{accountRepo: repo}
	account := newFableRecoveryAccount(anthropicFableWindowReason, nil)

	// 缺失不等于 0：上游没给 7d_oi 用量时不得撤销。
	svc.samplePassiveUsageFromHeaders(ctx, account, fableUsageHeaders("", false))

	require.Empty(t, repo.clearedScopes)
	require.True(t, account.isModelRateLimitedWithContext(ctx, "claude-fable-5-2"))
}

func TestPassiveUsageKeepsFableLimitWhileWindowStillRejected(t *testing.T) {
	ctx := context.Background()
	repo := &fableRecoveryRepo{}
	svc := &RateLimitService{accountRepo: repo}
	account := newFableRecoveryAccount(anthropicFableWindowReason, nil)

	// 窗口仍是 rejected 时，即使 utilization 因为口径差异低于 1.0 也不能撤销。
	svc.samplePassiveUsageFromHeaders(ctx, account, fableUsageHeaders("0.5", true))

	require.Empty(t, repo.clearedScopes)
	require.True(t, account.isModelRateLimitedWithContext(ctx, "claude-fable-5-2"))
}

func TestPassiveUsageKeepsFableLimitWhenUtilizationStillHigh(t *testing.T) {
	ctx := context.Background()
	repo := &fableRecoveryRepo{}
	svc := &RateLimitService{accountRepo: repo}
	account := newFableRecoveryAccount(anthropicFableWindowReason, nil)

	svc.samplePassiveUsageFromHeaders(ctx, account, fableUsageHeaders("0.98", false))

	require.Empty(t, repo.clearedScopes, "利用率仍接近耗尽线时不撤销")
}

func TestPassiveUsageKeepsCreditsRequiredLimit(t *testing.T) {
	ctx := context.Background()
	repo := &fableRecoveryRepo{}
	svc := &RateLimitService{accountRepo: repo}
	account := newFableRecoveryAccount(anthropicFableCreditsRequiredReason, nil)

	// 积分不足（credits required）与窗口耗尽语义不同，不能被窗口用量顺手清掉。
	svc.samplePassiveUsageFromHeaders(ctx, account, fableUsageHeaders("0", false))

	require.Empty(t, repo.clearedScopes)
	require.True(t, account.isModelRateLimitedWithContext(ctx, "claude-fable-5-2"))
}

func TestPassiveUsageKeepsOtherModelLimitsWhenRecoveringFable(t *testing.T) {
	ctx := context.Background()
	repo := &fableRecoveryRepo{}
	svc := &RateLimitService{accountRepo: repo}
	otherLimit := map[string]any{
		"rate_limited_at":     time.Now().Add(-time.Hour).UTC().Format(time.RFC3339),
		"rate_limit_reset_at": time.Now().Add(6 * time.Hour).UTC().Format(time.RFC3339),
		"reason":              anthropicFableWindowReason,
	}
	account := newFableRecoveryAccount(anthropicFableWindowReason, map[string]any{
		"claude-opus-4-1": otherLimit,
	})

	svc.samplePassiveUsageFromHeaders(ctx, account, fableUsageHeaders("0", false))

	require.Equal(t, []string{anthropicFableRateLimitKey}, repo.clearedScopes)
	require.False(t, account.isRateLimitActiveForKey(anthropicFableRateLimitKey))
	require.True(t, account.isRateLimitActiveForKey("claude-opus-4-1"),
		"其它模型的限流必须原样保留")
}

func TestPassiveUsageKeepsLimitWhenClearFails(t *testing.T) {
	ctx := context.Background()
	repo := &fableRecoveryRepo{clearErr: errors.New("db unavailable")}
	svc := &RateLimitService{accountRepo: repo}
	account := newFableRecoveryAccount(anthropicFableWindowReason, nil)

	svc.samplePassiveUsageFromHeaders(ctx, account, fableUsageHeaders("0", false))

	require.Equal(t, []string{anthropicFableRateLimitKey}, repo.clearedScopes)
	require.True(t, account.isModelRateLimitedWithContext(ctx, "claude-fable-5-2"),
		"写库失败时不能只改内存快照，否则本实例放行而调度快照仍拦截")
}

// 主动用量刷新（管理端「刷新额度」）是另一个观测点：上游 usage API 明确给出
// 已回落的 7d_oi 用量时同样要撤销；API 没给这个窗口时保持原状。
func TestSyncActiveToPassiveClearsExhaustedFableLimit(t *testing.T) {
	ctx := context.Background()
	repo := &fableRecoveryRepo{}
	svc := &AccountUsageService{accountRepo: repo}

	t.Run("api reports recovery", func(t *testing.T) {
		account := newFableRecoveryAccount(anthropicFableWindowReason, nil)
		usage := &UsageInfo{SevenDayFable: &UsageProgress{Utilization: 3}}

		svc.syncActiveToPassive(ctx, account, usage)

		require.Equal(t, []string{anthropicFableRateLimitKey}, repo.clearedScopes)
		require.False(t, account.isRateLimitActiveForKey(anthropicFableRateLimitKey))
	})

	t.Run("api omits the fable window", func(t *testing.T) {
		repo.clearedScopes = nil
		account := newFableRecoveryAccount(anthropicFableWindowReason, nil)
		usage := &UsageInfo{SevenDayFable: nil}

		svc.syncActiveToPassive(ctx, account, usage)

		require.Empty(t, repo.clearedScopes, "上游未下发该窗口时不能当成 0")
		require.True(t, account.isRateLimitActiveForKey(anthropicFableRateLimitKey))
	})
}
