//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func newSettingServiceForPlatformThresholdTest(seed map[string]string) *SettingService {
	accountSchedulingThresholdsSF.Forget(SettingKeyAccountSchedulingThresholds)
	accountSchedulingThresholdsCache.Store(&cachedAccountSchedulingThresholds{})
	repo := newMockSettingRepo()
	for k, v := range seed {
		repo.data[k] = v
	}
	return NewSettingService(repo, &config.Config{})
}

func TestPlatformSchedulingThresholds_RoundTrip_DefaultsAndStoredValues(t *testing.T) {
	svc := newSettingServiceForPlatformThresholdTest(nil)

	got := svc.parseSettings(map[string]string{})
	require.Equal(t, map[string]int{
		PlatformOpenAI:    100,
		PlatformAnthropic: 100,
		PlatformGrok:      100,
	}, got.AccountSchedulingThresholds)

	got = svc.parseSettings(map[string]string{
		SettingKeyAccountSchedulingThresholds: `{"openai":91,"grok":77,"gemini":85,"kiro":99}`,
	})
	require.Equal(t, 91, got.AccountSchedulingThresholds[PlatformOpenAI])
	require.Equal(t, 100, got.AccountSchedulingThresholds[PlatformAnthropic])
	require.Equal(t, 77, got.AccountSchedulingThresholds[PlatformGrok])
	require.NotContains(t, got.AccountSchedulingThresholds, PlatformGemini)
	require.NotContains(t, got.AccountSchedulingThresholds, "kiro")
}

func TestBuildSystemSettingsUpdates_PersistsAccountSchedulingThresholds(t *testing.T) {
	svc := newSettingServiceForPlatformThresholdTest(nil)

	updates, err := svc.buildSystemSettingsUpdates(context.Background(), &SystemSettings{
		AccountSchedulingThresholds: map[string]int{
			PlatformOpenAI:    91,
			PlatformAnthropic: 88,
			PlatformGrok:      77,
		},
	})
	require.NoError(t, err)
	require.JSONEq(t, `{"openai":91,"anthropic":88,"grok":77}`, updates[SettingKeyAccountSchedulingThresholds])
}

func TestValidateAndNormalizeAccountSchedulingThresholds_FillsMissingPlatforms(t *testing.T) {
	normalized, err := validateAndNormalizeAccountSchedulingThresholds(map[string]int{
		PlatformOpenAI: 91,
	})
	require.NoError(t, err)
	require.Equal(t, 91, normalized[PlatformOpenAI])
	require.Equal(t, 100, normalized[PlatformAnthropic])
	require.Equal(t, 100, normalized[PlatformGrok])
	require.NotContains(t, normalized, PlatformGemini)
	require.NotContains(t, normalized, "kiro")
	require.NotContains(t, normalized, PlatformAntigravity)
}

func TestValidateAndNormalizeAccountSchedulingThresholds_RejectsUnsupportedPlatforms(t *testing.T) {
	_, err := validateAndNormalizeAccountSchedulingThresholds(map[string]int{
		PlatformGemini: 85,
	})
	require.Error(t, err)
}

func TestUpdateSettings_StoresAccountSchedulingThresholds(t *testing.T) {
	svc := newSettingServiceForPlatformThresholdTest(nil)

	err := svc.UpdateSettings(context.Background(), &SystemSettings{
		AccountSchedulingThresholds: map[string]int{
			PlatformOpenAI:    92,
			PlatformAnthropic: 89,
			PlatformGrok:      76,
		},
	})
	require.NoError(t, err)

	got := svc.parseSettings(map[string]string{
		SettingKeyAccountSchedulingThresholds: svc.settingRepo.(*mockSettingRepo).data[SettingKeyAccountSchedulingThresholds],
	})
	require.Equal(t, 92, got.AccountSchedulingThresholds[PlatformOpenAI])
	require.Equal(t, 89, got.AccountSchedulingThresholds[PlatformAnthropic])
	require.Equal(t, 76, got.AccountSchedulingThresholds[PlatformGrok])
	require.NotContains(t, got.AccountSchedulingThresholds, "kiro")
}

func TestGetAccountSchedulingThresholds_ReadsStoredValue(t *testing.T) {
	svc := newSettingServiceForPlatformThresholdTest(map[string]string{
		SettingKeyAccountSchedulingThresholds: `{"openai":93,"grok":88,"kiro":87}`,
	})

	got := svc.GetAccountSchedulingThresholds(context.Background())

	require.Equal(t, 93, got[PlatformOpenAI])
	require.Equal(t, 100, got[PlatformAnthropic])
	require.Equal(t, 88, got[PlatformGrok])
	require.NotContains(t, got, "kiro")
}

func TestUpdateSettings_OmittedAccountSchedulingThresholdsDoesNotCacheDefaults(t *testing.T) {
	svc := newSettingServiceForPlatformThresholdTest(map[string]string{
		SettingKeyAccountSchedulingThresholds: `{"openai":85,"grok":88,"kiro":87}`,
	})

	err := svc.UpdateSettings(context.Background(), &SystemSettings{
		FrontendURL: "https://example.test",
	})
	require.NoError(t, err)

	got := svc.GetAccountSchedulingThresholds(context.Background())
	require.Equal(t, 85, got[PlatformOpenAI])
	require.Equal(t, 88, got[PlatformGrok])
	require.NotContains(t, got, "kiro")
}

func TestAccountSchedulingThresholds_InvalidStoredValueUsesSameDefaultsInSettingsAndCache(t *testing.T) {
	svc := newSettingServiceForPlatformThresholdTest(map[string]string{
		SettingKeyAccountSchedulingThresholds: `{"openai":0,"grok":88,"kiro":87}`,
	})

	settings := svc.parseSettings(map[string]string{
		SettingKeyAccountSchedulingThresholds: `{"openai":0,"grok":88,"kiro":87}`,
	})
	cached := svc.GetAccountSchedulingThresholds(context.Background())

	require.Equal(t, settings.AccountSchedulingThresholds, cached)
	require.Equal(t, 100, cached[PlatformOpenAI])
	require.Equal(t, 88, cached[PlatformGrok])
	require.NotContains(t, cached, "kiro")
}

func TestGetAccountSchedulingThresholds_NilRepoReturnsDefaults(t *testing.T) {
	svc := &SettingService{}
	got := svc.GetAccountSchedulingThresholds(context.Background())
	require.Equal(t, map[string]int{
		PlatformOpenAI:    100,
		PlatformAnthropic: 100,
		PlatformGrok:      100,
	}, got)
}
