//go:build unit

package service

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

type billingRiskStartupRepoStub struct {
	values           map[string]string
	getMultipleCalls atomic.Int64
}

func (s *billingRiskStartupRepoStub) Get(context.Context, string) (*Setting, error) {
	return nil, ErrSettingNotFound
}

func (s *billingRiskStartupRepoStub) GetValue(_ context.Context, key string) (string, error) {
	value, ok := s.values[key]
	if !ok {
		return "", ErrSettingNotFound
	}
	return value, nil
}

func (s *billingRiskStartupRepoStub) Set(_ context.Context, key, value string) error {
	s.values[key] = value
	return nil
}

func (s *billingRiskStartupRepoStub) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	s.getMultipleCalls.Add(1)
	values := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := s.values[key]; ok {
			values[key] = value
		}
	}
	return values, nil
}

func (s *billingRiskStartupRepoStub) SetMultiple(_ context.Context, values map[string]string) error {
	for key, value := range values {
		s.values[key] = value
	}
	return nil
}

func (s *billingRiskStartupRepoStub) GetAll(context.Context) (map[string]string, error) {
	values := make(map[string]string, len(s.values))
	for key, value := range s.values {
		values[key] = value
	}
	return values, nil
}

func (s *billingRiskStartupRepoStub) Delete(_ context.Context, key string) error {
	delete(s.values, key)
	return nil
}

func validBillingRiskSystemSettings() *SystemSettings {
	return &SystemSettings{
		BillingRiskEnabled:                  true,
		BillingRiskLowBalanceThreshold:      10,
		BillingRiskSafetyFactor:             1.25,
		BillingRiskMinimumRequestRisk:       0.001,
		BillingRiskOverdraftAllowance:       0.20,
		BillingRiskHighCostTrigger:          1,
		BillingRiskLeaseTTLSeconds:          60,
		BillingRiskRefreshIntervalSeconds:   15,
		BillingRiskUncertainCooldownSeconds: 300,
		BillingRiskVideoLeaseTTLSeconds:     86400,
		BillingRiskIdleBalanceTTLSeconds:    120,
	}
}

func TestBillingRiskSettingsDefaults(t *testing.T) {
	svc := NewSettingService(&settingGetAllRepoStub{values: map[string]string{}}, &config.Config{})

	settings, err := svc.GetAllSettings(context.Background())
	require.NoError(t, err)
	expected := validBillingRiskSystemSettings()
	expected.BillingRiskEnabled = false
	require.Equal(t, expected, &SystemSettings{
		BillingRiskEnabled:                  settings.BillingRiskEnabled,
		BillingRiskLowBalanceThreshold:      settings.BillingRiskLowBalanceThreshold,
		BillingRiskSafetyFactor:             settings.BillingRiskSafetyFactor,
		BillingRiskMinimumRequestRisk:       settings.BillingRiskMinimumRequestRisk,
		BillingRiskOverdraftAllowance:       settings.BillingRiskOverdraftAllowance,
		BillingRiskHighCostTrigger:          settings.BillingRiskHighCostTrigger,
		BillingRiskLeaseTTLSeconds:          settings.BillingRiskLeaseTTLSeconds,
		BillingRiskRefreshIntervalSeconds:   settings.BillingRiskRefreshIntervalSeconds,
		BillingRiskUncertainCooldownSeconds: settings.BillingRiskUncertainCooldownSeconds,
		BillingRiskVideoLeaseTTLSeconds:     settings.BillingRiskVideoLeaseTTLSeconds,
		BillingRiskIdleBalanceTTLSeconds:    settings.BillingRiskIdleBalanceTTLSeconds,
	})
	require.Equal(t, DefaultBillingRiskSettings(), svc.GetBillingRiskSettings())
}

func TestProvideSettingServiceLoadsBillingRiskSettingsAtStartup(t *testing.T) {
	repo := &billingRiskStartupRepoStub{values: map[string]string{
		SettingKeyBillingRiskEnabled:             "false",
		SettingKeyBillingRiskLowBalanceThreshold: "7.5",
	}}

	svc := ProvideSettingService(repo, nil, nil, &config.Config{})

	require.False(t, svc.GetBillingRiskSettings().Enabled)
	require.Equal(t, 7.5, svc.GetBillingRiskSettings().LowBalanceThreshold)
}

func TestBillingRiskSettingsReloadAcrossServiceInstancesAfterCacheExpiry(t *testing.T) {
	repo := &billingRiskStartupRepoStub{values: map[string]string{
		SettingKeyBillingRiskEnabled: "true",
	}}
	updatingInstance := NewSettingService(repo, &config.Config{})
	staleInstance := NewSettingService(repo, &config.Config{})
	require.NoError(t, updatingInstance.LoadBillingRiskSettings(context.Background()))
	require.NoError(t, staleInstance.LoadBillingRiskSettings(context.Background()))

	updated := validBillingRiskSystemSettings()
	updated.BillingRiskEnabled = false
	require.NoError(t, updatingInstance.UpdateSettings(context.Background(), updated))
	require.False(t, updatingInstance.GetBillingRiskSettings().Enabled)
	require.True(t, staleInstance.GetBillingRiskSettings().Enabled)

	cached := staleInstance.billingRiskSettingsCache.Load().(*cachedBillingRiskSettings)
	staleInstance.billingRiskSettingsCache.Store(&cachedBillingRiskSettings{
		settings:  cached.settings,
		expiresAt: time.Now().Add(-time.Second).UnixNano(),
	})
	repo.getMultipleCalls.Store(0)
	require.True(t, staleInstance.GetBillingRiskSettings().Enabled)
	for range 31 {
		_ = staleInstance.GetBillingRiskSettings()
	}

	require.Eventually(t, func() bool {
		return !staleInstance.GetBillingRiskSettings().Enabled
	}, time.Second, 10*time.Millisecond)
	require.Equal(t, int64(1), repo.getMultipleCalls.Load())
}

func TestBillingRiskSettingsPersistAndRefreshImmediately(t *testing.T) {
	repo := &settingUpdateRepoStub{}
	svc := NewSettingService(repo, &config.Config{})
	settings := validBillingRiskSystemSettings()
	settings.BillingRiskLowBalanceThreshold = 8.5
	settings.BillingRiskSafetyFactor = 1.5
	settings.BillingRiskMinimumRequestRisk = 0.002
	settings.BillingRiskOverdraftAllowance = 0.3
	settings.BillingRiskHighCostTrigger = 2
	settings.BillingRiskLeaseTTLSeconds = 90
	settings.BillingRiskRefreshIntervalSeconds = 20
	settings.BillingRiskUncertainCooldownSeconds = 420
	settings.BillingRiskVideoLeaseTTLSeconds = 172800
	settings.BillingRiskIdleBalanceTTLSeconds = 180

	require.NoError(t, svc.UpdateSettings(context.Background(), settings))
	require.Equal(t, "true", repo.updates[SettingKeyBillingRiskEnabled])
	require.Equal(t, "8.5", repo.updates[SettingKeyBillingRiskLowBalanceThreshold])
	require.Equal(t, "1.5", repo.updates[SettingKeyBillingRiskSafetyFactor])
	require.Equal(t, "0.002", repo.updates[SettingKeyBillingRiskMinimumRequestRisk])
	require.Equal(t, "0.3", repo.updates[SettingKeyBillingRiskOverdraftAllowance])
	require.Equal(t, "2", repo.updates[SettingKeyBillingRiskHighCostTrigger])
	require.Equal(t, "90", repo.updates[SettingKeyBillingRiskLeaseTTLSeconds])
	require.Equal(t, "20", repo.updates[SettingKeyBillingRiskRefreshIntervalSeconds])
	require.Equal(t, "420", repo.updates[SettingKeyBillingRiskUncertainCooldownSeconds])
	require.Equal(t, "172800", repo.updates[SettingKeyBillingRiskVideoLeaseTTLSeconds])
	require.Equal(t, "180", repo.updates[SettingKeyBillingRiskIdleBalanceTTLSeconds])

	require.Equal(t, BillingRiskSettings{
		Enabled:                  true,
		LowBalanceThreshold:      8.5,
		SafetyFactor:             1.5,
		MinimumRequestRisk:       0.002,
		OverdraftAllowance:       0.3,
		HighCostTrigger:          2,
		LeaseTTLSeconds:          90,
		RefreshIntervalSeconds:   20,
		UncertainCooldownSeconds: 420,
		VideoLeaseTTLSeconds:     172800,
		IdleBalanceTTLSeconds:    180,
	}, svc.GetBillingRiskSettings())
}

func TestBillingRiskSettingsRejectInvalidValues(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*SystemSettings)
	}{
		{name: "余额阈值为负", mutate: func(s *SystemSettings) { s.BillingRiskLowBalanceThreshold = -1 }},
		{name: "安全系数小于一", mutate: func(s *SystemSettings) { s.BillingRiskSafetyFactor = 0.99 }},
		{name: "最小风险为负", mutate: func(s *SystemSettings) { s.BillingRiskMinimumRequestRisk = -0.001 }},
		{name: "透支额度为负", mutate: func(s *SystemSettings) { s.BillingRiskOverdraftAllowance = -0.1 }},
		{name: "高成本阈值为负", mutate: func(s *SystemSettings) { s.BillingRiskHighCostTrigger = -1 }},
		{name: "租约不足两个刷新周期", mutate: func(s *SystemSettings) {
			s.BillingRiskLeaseTTLSeconds = 30
			s.BillingRiskRefreshIntervalSeconds = 15
		}},
		{name: "视频租约短于普通租约", mutate: func(s *SystemSettings) { s.BillingRiskVideoLeaseTTLSeconds = 30 }},
		{name: "异常冷却非正数", mutate: func(s *SystemSettings) { s.BillingRiskUncertainCooldownSeconds = 0 }},
		{name: "空闲余额 TTL 非正数", mutate: func(s *SystemSettings) { s.BillingRiskIdleBalanceTTLSeconds = 0 }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &settingUpdateRepoStub{}
			svc := NewSettingService(repo, &config.Config{})
			settings := validBillingRiskSystemSettings()
			tt.mutate(settings)

			err := svc.UpdateSettings(context.Background(), settings)
			require.Error(t, err)
			require.Equal(t, "INVALID_BILLING_RISK_SETTINGS", infraerrors.Reason(err))
			require.Nil(t, repo.updates)
		})
	}
}
