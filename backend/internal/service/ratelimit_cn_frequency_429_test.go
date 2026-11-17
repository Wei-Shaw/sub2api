//go:build unit

package service

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

// 智谱 GLM Coding Plan 实测 429 文案：额度充裕时的「限频」。
const zhipuFrequencyLimitedBody = `{"error":{"code":"1302","message":"您的账户已达到速率限制，请您控制请求频率"}}`

// newZhipuCodingPlanAccount 构造一个智谱 Coding Plan 账号，Extra 带 5h/weekly
// 用量与重置快照（与 CNProviderQuotaService 写入的键一致）。
func newZhipuCodingPlanAccount(h5Used, weeklyUsed float64, resetIn time.Duration) *Account {
	extra := map[string]any{
		cnExtraKey(PlatformZhipu, cnExtraSuffix5hUsed):     h5Used,
		cnExtraKey(PlatformZhipu, cnExtraSuffixWeeklyUsed): weeklyUsed,
	}
	if resetIn > 0 {
		extra[cnExtraKey(PlatformZhipu, cnExtraSuffix5hReset)] = time.Now().Add(resetIn).UTC().Format(time.RFC3339)
		extra[cnExtraKey(PlatformZhipu, cnExtraSuffixWeeklyReset)] = time.Now().Add(4 * 24 * time.Hour).UTC().Format(time.RFC3339)
	}
	return &Account{
		ID:          701,
		Platform:    PlatformZhipu,
		Type:        AccountTypeAPIKey,
		Credentials: map[string]any{"account_mode": AccountModeCoding},
		Extra:       extra,
	}
}

func runZhipu429(service *RateLimitService, account *Account, body string) bool {
	return service.HandleUpstreamError(
		context.Background(),
		account,
		http.StatusTooManyRequests,
		http.Header{},
		[]byte(body),
	)
}

// 额度充裕 + 限频文案 → 走秒级 429 兜底，而不是冷却到 5h 窗口结束。
func TestHandleUpstreamError_ZhipuFrequency429UsesShortCooldown(t *testing.T) {
	repo := &rateLimitAccountRepoStub{}
	service := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	account := newZhipuCodingPlanAccount(12, 4, 4*time.Hour)

	runZhipu429(service, account, zhipuFrequencyLimitedBody)

	require.Equal(t, 0, repo.setErrorCalls, "a frequency 429 must never SetError")
	require.Equal(t, 1, repo.rateLimitedCalls)
	require.Equal(t, account.ID, repo.lastRateLimitedID)
	require.WithinDuration(t, time.Now().Add(defaultRateLimit429CooldownSeconds*time.Second),
		repo.lastRateLimitedAt, 30*time.Second,
		"frequency 429 must cool down seconds, not to the 5h window reset")
}

// 快照显示窗口已耗尽时，即使文案是限频，也仍然冷却到窗口重置点（护栏）。
func TestHandleUpstreamError_ZhipuFrequency429WithExhaustedSnapshotKeepsWindowCooldown(t *testing.T) {
	repo := &rateLimitAccountRepoStub{}
	service := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	account := newZhipuCodingPlanAccount(100, 40, 2*time.Hour)

	runZhipu429(service, account, zhipuFrequencyLimitedBody)

	require.Equal(t, 0, repo.setErrorCalls)
	require.Equal(t, 1, repo.rateLimitedCalls)
	require.True(t, repo.lastRateLimitedAt.After(time.Now().Add(time.Hour)),
		"exhausted snapshot must keep cooling down to the window reset")
}

// 额度类文案（即使同时出现"速率限制"字样）不降级，仍按窗口冷却。
func TestHandleUpstreamError_ZhipuQuotaWorded429KeepsWindowCooldown(t *testing.T) {
	repo := &rateLimitAccountRepoStub{}
	service := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	account := newZhipuCodingPlanAccount(12, 4, 2*time.Hour)
	body := `{"error":{"message":"已达速率限制：本月额度已用尽，请升级套餐"}}`

	runZhipu429(service, account, body)

	require.Equal(t, 1, repo.rateLimitedCalls)
	require.True(t, repo.lastRateLimitedAt.After(time.Now().Add(time.Hour)),
		"quota wording must not be downgraded to the short 429 cooldown")
}

// 余额不足文案仍走原有的临时停调路径，不受本次改动影响。
func TestHandleUpstreamError_ZhipuInsufficientBalance429StillTempUnschedulable(t *testing.T) {
	repo := &rateLimitAccountRepoStub{}
	service := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	account := newZhipuCodingPlanAccount(12, 4, 4*time.Hour)

	runZhipu429(service, account, `{"error":{"message":"余额不足，请充值"}}`)

	require.Equal(t, 0, repo.rateLimitedCalls)
	require.Equal(t, 1, repo.tempCalls)
	require.Contains(t, repo.lastTempReason, cnBalanceLowReasonPrefix)
}

func TestCNProviderResponseIndicatesFrequencyLimit(t *testing.T) {
	cases := []struct {
		name string
		body string
		want bool
	}{
		{"zhipu frequency wording", zhipuFrequencyLimitedBody, true},
		{"generic too many requests", `{"error":{"message":"Too many requests, please slow down"}}`, true},
		{"openai style rate limit", `{"error":{"code":"rate_limit_exceeded","message":"Rate limit exceeded"}}`, true},
		{"quota wording wins", `{"error":{"message":"已达到速率限制，但本月额度已用尽"}}`, false},
		{"usage limit wording wins", `{"error":{"message":"You've reached your weekly usage limit"}}`, false},
		{"insufficient balance wins", `{"error":{"message":"insufficient balance"}}`, false},
		{"unrelated error", `{"error":{"message":"model not found"}}`, false},
		{"empty body", ``, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, cnProviderResponseIndicatesFrequencyLimit([]byte(tc.body)))
		})
	}
}

func TestCNProviderQuotaSnapshotNearExhausted(t *testing.T) {
	require.False(t, cnProviderQuotaSnapshotNearExhausted(
		newZhipuCodingPlanAccount(12, 4, time.Hour), cnFrequencyLimitQuotaGuardPercent))
	require.True(t, cnProviderQuotaSnapshotNearExhausted(
		newZhipuCodingPlanAccount(90, 4, time.Hour), cnFrequencyLimitQuotaGuardPercent))
	require.True(t, cnProviderQuotaSnapshotNearExhausted(
		newZhipuCodingPlanAccount(12, 95, time.Hour), cnFrequencyLimitQuotaGuardPercent))

	// 快照缺失：无法证明与额度无关 → 保守视为接近耗尽（沿用窗口冷却）。
	require.True(t, cnProviderQuotaSnapshotNearExhausted(
		&Account{ID: 702, Platform: PlatformZhipu}, cnFrequencyLimitQuotaGuardPercent))
	require.True(t, cnProviderQuotaSnapshotNearExhausted(nil, cnFrequencyLimitQuotaGuardPercent))
}
