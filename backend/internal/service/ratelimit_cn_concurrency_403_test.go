//go:build unit

package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type kimiConcurrencyRateLimitFailureRepo struct {
	rateLimitAccountRepoStub
}

func (repo *kimiConcurrencyRateLimitFailureRepo) SetRateLimited(ctx context.Context, accountID int64, resetAt time.Time) error {
	_ = repo.rateLimitAccountRepoStub.SetRateLimited(ctx, accountID, resetAt)
	return errors.New("repository unavailable")
}

func TestHandleUpstreamError_KimiConcurrency403CooldownSettings(t *testing.T) {
	for _, testCase := range []struct {
		name     string
		settings *RateLimit429CooldownSettings
		seconds  int
	}{
		{name: "no setting service", seconds: 5},
		{name: "default settings", settings: &RateLimit429CooldownSettings{}, seconds: 5},
		{name: "custom seconds", settings: &RateLimit429CooldownSettings{Enabled: true, CooldownSeconds: 17}, seconds: 17},
		{name: "disabled", settings: &RateLimit429CooldownSettings{Enabled: false, CooldownSeconds: 17}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			repo := &rateLimitAccountRepoStub{}
			counter := &openAI403CounterCacheStub{counts: []int64{openAI403DisableThreshold}}
			blocker := &runtimeBlockRecorder{}
			service := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
			service.SetOpenAI403CounterCache(counter)
			service.SetAccountRuntimeBlocker(blocker)
			if testCase.settings != nil {
				settingRepo := newMockSettingRepo()
				if testCase.settings.CooldownSeconds > 0 {
					data, err := json.Marshal(testCase.settings)
					require.NoError(t, err)
					settingRepo.data[SettingKeyRateLimit429CooldownSettings] = string(data)
				}
				service.SetSettingService(NewSettingService(settingRepo, &config.Config{}))
			}
			account := newCNQuotaExhaustedTestAccount(2 * time.Hour)
			for attempt := 0; attempt < 5; attempt++ {
				before := time.Now()
				require.True(t, runKimi403(service, account, `{"error":{"message":"`+kimiConcurrentRequestLimitMessage+`"}}`), "must switch accounts even with cooldown disabled")
				after := time.Now()
				if testCase.seconds > 0 {
					cooldown := time.Duration(testCase.seconds) * time.Second
					require.Equal(t, attempt+1, repo.rateLimitedCalls)
					require.Equal(t, account.ID, repo.lastRateLimitedID)
					require.False(t, repo.lastRateLimitedAt.Before(before.Add(cooldown)))
					require.False(t, repo.lastRateLimitedAt.After(after.Add(cooldown)))
					require.Len(t, blocker.accounts, attempt+1)
					require.Same(t, account, blocker.accounts[attempt])
					require.Equal(t, repo.lastRateLimitedAt, blocker.until[attempt])
					require.Equal(t, "429_fallback", blocker.reasons[attempt])
				} else {
					require.Zero(t, repo.rateLimitedCalls)
					require.Empty(t, blocker.accounts)
				}
				require.Zero(t, repo.tempCalls)
				require.Zero(t, repo.setErrorCalls)
				require.Equal(t, []int64{openAI403DisableThreshold}, counter.counts)
			}
		})
	}
}

func TestHandleUpstreamError_KimiQuota403IgnoresDisabledConcurrencyCooldown(t *testing.T) {
	for _, resetIn := range []time.Duration{0, 2 * time.Hour} {
		t.Run(resetIn.String(), func(t *testing.T) {
			repo := &rateLimitAccountRepoStub{}
			blocker := &runtimeBlockRecorder{}
			service := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
			service.SetAccountRuntimeBlocker(blocker)
			settingRepo := newMockSettingRepo()
			settingRepo.data[SettingKeyRateLimit429CooldownSettings] = `{"enabled":false,"cooldown_seconds":17}`
			service.SetSettingService(NewSettingService(settingRepo, &config.Config{}))
			account := newCNQuotaExhaustedTestAccount(resetIn)
			require.True(t, runKimi403(service, account, kimiWeeklyQuotaExhaustedBody))
			require.Zero(t, repo.setErrorCalls)
			require.Len(t, blocker.accounts, 1)
			require.Equal(t, cnQuotaExhaustedReasonPrefix, blocker.reasons[0])
			if resetIn > 0 {
				require.Equal(t, 1, repo.rateLimitedCalls)
				require.Zero(t, repo.tempCalls)
				require.WithinDuration(t, time.Now().Add(resetIn), repo.lastRateLimitedAt, time.Second)
			} else {
				require.Zero(t, repo.rateLimitedCalls)
				require.Equal(t, 1, repo.tempCalls)
				require.WithinDuration(t, time.Now().Add(10*time.Minute), blocker.until[0], time.Second)
			}
		})
	}
}

func TestHandleUpstreamError_KimiConcurrency403ExplicitRulesTakePriority(t *testing.T) {
	for _, testCase := range []struct {
		name        string
		credentials map[string]any
		wantSwitch  bool
		wantTemp    int
		wantLimited int
	}{
		{
			name: "custom error codes exclude 403",
			credentials: map[string]any{
				"custom_error_codes_enabled": true,
				"custom_error_codes":         []any{float64(429)},
			},
		},
		{
			name: "custom error codes include 403",
			credentials: map[string]any{
				"custom_error_codes_enabled": true,
				"custom_error_codes":         []any{float64(403)},
			},
			wantSwitch: true, wantLimited: 1,
		},
		{
			name: "explicit temporary unschedulable rule",
			credentials: map[string]any{
				"temp_unschedulable_enabled": true,
				"temp_unschedulable_rules": []any{map[string]any{
					"error_code": float64(403), "keywords": []any{"concurrent request limit"},
					"duration_minutes": float64(23),
				}},
			},
			wantSwitch: true, wantTemp: 1,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			repo := &rateLimitAccountRepoStub{}
			blocker := &runtimeBlockRecorder{}
			counter := &openAI403CounterCacheStub{counts: []int64{openAI403DisableThreshold}}
			service := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
			service.SetAccountRuntimeBlocker(blocker)
			service.SetOpenAI403CounterCache(counter)
			account := newCNQuotaExhaustedTestAccount(0)
			account.Credentials = testCase.credentials
			before := time.Now()
			require.Equal(t, testCase.wantSwitch, runKimi403(service, account,
				`{"error":{"message":"`+kimiConcurrentRequestLimitMessage+`"}}`))
			require.Equal(t, testCase.wantTemp, repo.tempCalls)
			require.Equal(t, testCase.wantLimited, repo.rateLimitedCalls)
			require.Zero(t, repo.setErrorCalls)
			require.Equal(t, []int64{openAI403DisableThreshold}, counter.counts)
			if testCase.wantTemp > 0 {
				require.Len(t, blocker.accounts, 1)
				require.NotEqual(t, "429_fallback", blocker.reasons[0])
				require.WithinDuration(t, before.Add(23*time.Minute), blocker.until[0], time.Second)
			} else if !testCase.wantSwitch {
				require.Empty(t, blocker.accounts)
			}
		})
	}
}
