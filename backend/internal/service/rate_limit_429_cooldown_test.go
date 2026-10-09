//go:build unit

package service

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type rateLimit429AccountRepoStub struct {
	mockAccountRepoForGemini
	rateLimitCalls     int
	lastRateLimitID    int64
	lastRateLimitReset time.Time
	tempUnschedCalls   int
	lastTempUntil      time.Time
	lastTempReason     string
}

func (r *rateLimit429AccountRepoStub) SetRateLimited(_ context.Context, id int64, resetAt time.Time) error {
	r.rateLimitCalls++
	r.lastRateLimitID = id
	r.lastRateLimitReset = resetAt
	return nil
}

func (r *rateLimit429AccountRepoStub) SetTempUnschedulable(_ context.Context, _ int64, until time.Time, reason string) error {
	r.tempUnschedCalls++
	r.lastTempUntil = until
	r.lastTempReason = reason
	return nil
}

func TestGetRateLimit429CooldownSettings_DefaultsWhenNotSet(t *testing.T) {
	repo := newMockSettingRepo()
	svc := NewSettingService(repo, &config.Config{})

	settings, err := svc.GetRateLimit429CooldownSettings(context.Background())
	require.NoError(t, err)
	require.True(t, settings.Enabled)
	require.Equal(t, 5, settings.CooldownSeconds)
}

func TestGetRateLimit429CooldownSettings_ReadsFromDB(t *testing.T) {
	repo := newMockSettingRepo()
	data, _ := json.Marshal(RateLimit429CooldownSettings{Enabled: false, CooldownSeconds: 12})
	repo.data[SettingKeyRateLimit429CooldownSettings] = string(data)
	svc := NewSettingService(repo, &config.Config{})

	settings, err := svc.GetRateLimit429CooldownSettings(context.Background())
	require.NoError(t, err)
	require.False(t, settings.Enabled)
	require.Equal(t, 12, settings.CooldownSeconds)
}

func TestSetRateLimit429CooldownSettings_EnabledRejectsOutOfRange(t *testing.T) {
	svc := NewSettingService(newMockSettingRepo(), &config.Config{})

	for _, seconds := range []int{0, -1, 7201, 99999} {
		err := svc.SetRateLimit429CooldownSettings(context.Background(), &RateLimit429CooldownSettings{
			Enabled: true, CooldownSeconds: seconds,
		})
		require.Error(t, err, "should reject enabled=true + cooldown_seconds=%d", seconds)
		require.Contains(t, err.Error(), "cooldown_seconds must be between 1-7200")
	}
}

func TestHandle429_FallbackUsesDBSeconds(t *testing.T) {
	accountRepo := &rateLimit429AccountRepoStub{}
	settingRepo := newMockSettingRepo()
	data, _ := json.Marshal(RateLimit429CooldownSettings{Enabled: true, CooldownSeconds: 12})
	settingRepo.data[SettingKeyRateLimit429CooldownSettings] = string(data)

	settingSvc := NewSettingService(settingRepo, &config.Config{})
	svc := NewRateLimitService(accountRepo, nil, &config.Config{}, nil, nil)
	svc.SetSettingService(settingSvc)

	account := &Account{ID: 42, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	before := time.Now()
	svc.handle429(context.Background(), account, http.Header{}, []byte(`{"error":{"type":"rate_limit_error","message":"slow down"}}`))
	after := time.Now()

	require.Equal(t, 1, accountRepo.rateLimitCalls)
	require.Equal(t, int64(42), accountRepo.lastRateLimitID)
	require.True(t, !accountRepo.lastRateLimitReset.Before(before.Add(12*time.Second)) && !accountRepo.lastRateLimitReset.After(after.Add(12*time.Second)))
}

func TestHandle429_FallbackDisabledSkipsLocalMark(t *testing.T) {
	accountRepo := &rateLimit429AccountRepoStub{}
	settingRepo := newMockSettingRepo()
	data, _ := json.Marshal(RateLimit429CooldownSettings{Enabled: false, CooldownSeconds: 12})
	settingRepo.data[SettingKeyRateLimit429CooldownSettings] = string(data)

	settingSvc := NewSettingService(settingRepo, &config.Config{})
	svc := NewRateLimitService(accountRepo, nil, &config.Config{}, nil, nil)
	svc.SetSettingService(settingSvc)

	account := &Account{ID: 43, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	svc.handle429(context.Background(), account, http.Header{}, []byte(`{"error":{"type":"rate_limit_error","message":"slow down"}}`))

	require.Zero(t, accountRepo.rateLimitCalls)
}

func TestHandle429_KimiDynamicAllocation429UsesShortFallback(t *testing.T) {
	accountRepo := &rateLimit429AccountRepoStub{}
	settingRepo := newMockSettingRepo()
	data, _ := json.Marshal(RateLimit429CooldownSettings{Enabled: true, CooldownSeconds: 12})
	settingRepo.data[SettingKeyRateLimit429CooldownSettings] = string(data)

	settingSvc := NewSettingService(settingRepo, &config.Config{})
	svc := NewRateLimitService(accountRepo, nil, &config.Config{}, nil, nil)
	svc.SetSettingService(settingSvc)
	now := time.Now()
	account := &Account{
		ID: 47, Platform: PlatformKimi, Type: AccountTypeAPIKey,
		Credentials: map[string]any{"account_mode": AccountModeCoding},
		Extra: map[string]any{
			"kimi_5h_reset_at":     now.Add(2 * time.Hour).Format(time.RFC3339),
			"kimi_weekly_reset_at": now.Add(24 * time.Hour).Format(time.RFC3339),
		},
	}

	before := time.Now()
	svc.handle429(context.Background(), account, http.Header{}, []byte(`{"error":{"code":"Throttling.AllocationQuota","message":"usage allocated quota exceeded. please try again later"}}`))
	after := time.Now()

	// 动态资源限流不能因为快照里有未来窗口，就被冷却到两小时后的 5h reset。
	require.Equal(t, 1, accountRepo.rateLimitCalls)
	require.True(t, accountRepo.lastRateLimitReset.After(before.Add(12*time.Second)))
	require.True(t, accountRepo.lastRateLimitReset.Before(after.Add(12*time.Second)))
}

func TestHandle429_KimiExplicitWeeklyQuotaUsesWindowReset(t *testing.T) {
	accountRepo := &rateLimit429AccountRepoStub{}
	settingSvc := NewSettingService(newMockSettingRepo(), &config.Config{})
	svc := NewRateLimitService(accountRepo, nil, &config.Config{}, nil, nil)
	svc.SetSettingService(settingSvc)
	now := time.Now()
	reset := now.Add(3 * time.Hour)
	account := &Account{
		ID: 48, Platform: PlatformKimi, Type: AccountTypeAPIKey,
		Credentials: map[string]any{"account_mode": AccountModeCoding},
		Extra: map[string]any{
			"kimi_5h_reset_at":     reset.Format(time.RFC3339),
			"kimi_weekly_reset_at": now.Add(24 * time.Hour).Format(time.RFC3339),
		},
	}

	svc.handle429(context.Background(), account, http.Header{}, []byte(`{"error":{"message":"You've reached your weekly (7-day) usage limit. Your quota will reset when the current 7-day window ends."}}`))

	require.Equal(t, 1, accountRepo.rateLimitCalls)
	require.WithinDuration(t, reset, accountRepo.lastRateLimitReset, time.Second)
}

func TestHandleUpstreamError_KimiDynamic429UsesAccountTempRule(t *testing.T) {
	accountRepo := &rateLimit429AccountRepoStub{}
	settingSvc := NewSettingService(newMockSettingRepo(), &config.Config{})
	svc := NewRateLimitService(accountRepo, nil, &config.Config{}, nil, nil)
	svc.SetSettingService(settingSvc)
	now := time.Now()
	account := &Account{
		ID: 49, Platform: PlatformKimi, Type: AccountTypeAPIKey,
		Credentials: map[string]any{
			"account_mode":               AccountModeCoding,
			"temp_unschedulable_enabled": true,
			"temp_unschedulable_rules": []any{map[string]any{
				"error_code":       float64(http.StatusTooManyRequests),
				"keywords":         []any{"Throttling.AllocationQuota"},
				"duration_minutes": float64(60),
			}},
		},
		Extra: map[string]any{
			"kimi_5h_reset_at":     now.Add(2 * time.Hour).Format(time.RFC3339),
			"kimi_weekly_reset_at": now.Add(24 * time.Hour).Format(time.RFC3339),
		},
	}

	shouldDisable := svc.HandleUpstreamError(context.Background(), account, http.StatusTooManyRequests, http.Header{}, []byte(`{"error":{"code":"Throttling.AllocationQuota","message":"usage allocated quota exceeded"}}`))

	require.True(t, shouldDisable)
	require.Zero(t, accountRepo.rateLimitCalls, "动态 429 命中账号规则时不应写入窗口限流")
	require.Equal(t, 1, accountRepo.tempUnschedCalls)
	require.WithinDuration(t, time.Now().Add(60*time.Minute), accountRepo.lastTempUntil, 2*time.Second)
	require.Contains(t, accountRepo.lastTempReason, "Throttling.AllocationQuota")
}

func TestHandleUpstreamError_KimiExplicitQuotaBypassesBroadTempRule(t *testing.T) {
	accountRepo := &rateLimit429AccountRepoStub{}
	svc := NewRateLimitService(accountRepo, nil, &config.Config{}, nil, nil)
	svc.SetSettingService(NewSettingService(newMockSettingRepo(), &config.Config{}))
	now := time.Now()
	reset := now.Add(3 * time.Hour)
	account := &Account{
		ID: 50, Platform: PlatformKimi, Type: AccountTypeAPIKey,
		Credentials: map[string]any{
			"account_mode":               AccountModeCoding,
			"temp_unschedulable_enabled": true,
			"temp_unschedulable_rules": []any{map[string]any{
				"error_code":       float64(http.StatusTooManyRequests),
				"keywords":         []any{"usage limit"},
				"duration_minutes": float64(60),
			}},
		},
		Extra: map[string]any{
			"kimi_5h_reset_at":     reset.Format(time.RFC3339),
			"kimi_weekly_reset_at": now.Add(24 * time.Hour).Format(time.RFC3339),
		},
	}

	shouldDisable := svc.HandleUpstreamError(context.Background(), account, http.StatusTooManyRequests, http.Header{}, []byte(`{"error":{"message":"You've reached your weekly (7-day) usage limit. Your quota will reset when the current 7-day window ends."}}`))

	require.False(t, shouldDisable)
	require.Zero(t, accountRepo.tempUnschedCalls, "明确配额耗尽不应被账号临时规则缩短")
	require.Equal(t, 1, accountRepo.rateLimitCalls)
	require.WithinDuration(t, reset, accountRepo.lastRateLimitReset, time.Second)
}

// Anthropic 无 reset 头的 429（如 Extra usage required）也应走兜底冷却，
// 否则账号永不冷却，调度器会让每个请求反复撞同一批 429 账号（旋转木马）。
func TestHandle429_AnthropicNoResetTimeUsesFallbackCooldown(t *testing.T) {
	accountRepo := &rateLimit429AccountRepoStub{}
	settingRepo := newMockSettingRepo()
	data, _ := json.Marshal(RateLimit429CooldownSettings{Enabled: true, CooldownSeconds: 12})
	settingRepo.data[SettingKeyRateLimit429CooldownSettings] = string(data)

	settingSvc := NewSettingService(settingRepo, &config.Config{})
	svc := NewRateLimitService(accountRepo, nil, &config.Config{}, nil, nil)
	svc.SetSettingService(settingSvc)

	account := &Account{ID: 45, Platform: PlatformAnthropic, Type: AccountTypeOAuth}
	before := time.Now()
	svc.handle429(context.Background(), account, http.Header{}, []byte(`{"error":{"type":"rate_limit_error","message":"Extra usage required"}}`))
	after := time.Now()

	require.Equal(t, 1, accountRepo.rateLimitCalls)
	require.Equal(t, int64(45), accountRepo.lastRateLimitID)
	require.True(t, !accountRepo.lastRateLimitReset.Before(before.Add(12*time.Second)) && !accountRepo.lastRateLimitReset.After(after.Add(12*time.Second)))
}

// 管理端关闭兜底冷却时，Anthropic 无 reset 头的 429 保持旧行为：不标记账号。
func TestHandle429_AnthropicNoResetTimeFallbackDisabledSkipsMark(t *testing.T) {
	accountRepo := &rateLimit429AccountRepoStub{}
	settingRepo := newMockSettingRepo()
	data, _ := json.Marshal(RateLimit429CooldownSettings{Enabled: false, CooldownSeconds: 12})
	settingRepo.data[SettingKeyRateLimit429CooldownSettings] = string(data)

	settingSvc := NewSettingService(settingRepo, &config.Config{})
	svc := NewRateLimitService(accountRepo, nil, &config.Config{}, nil, nil)
	svc.SetSettingService(settingSvc)

	account := &Account{ID: 46, Platform: PlatformAnthropic, Type: AccountTypeOAuth}
	svc.handle429(context.Background(), account, http.Header{}, []byte(`{"error":{"type":"rate_limit_error","message":"Extra usage required"}}`))

	require.Zero(t, accountRepo.rateLimitCalls)
}

func TestHandle429_FallbackUsesDefaultSecondsWhenSettingServiceMissing(t *testing.T) {
	accountRepo := &rateLimit429AccountRepoStub{}
	cfg := &config.Config{}
	svc := NewRateLimitService(accountRepo, nil, cfg, nil, nil)

	account := &Account{ID: 44, Platform: PlatformGemini, Type: AccountTypeAPIKey}
	before := time.Now()
	svc.handle429(context.Background(), account, http.Header{}, []byte(`{"error":{"message":"slow down"}}`))
	after := time.Now()

	require.Equal(t, 1, accountRepo.rateLimitCalls)
	require.Equal(t, int64(44), accountRepo.lastRateLimitID)
	require.True(t, !accountRepo.lastRateLimitReset.Before(before.Add(5*time.Second)) && !accountRepo.lastRateLimitReset.After(after.Add(5*time.Second)))
}
