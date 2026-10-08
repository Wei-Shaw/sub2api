package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func resetOpenAICacheWriteInferenceSwitchForTest(t *testing.T) {
	t.Helper()
	gatewayForwardingSF.Forget("gateway_forwarding")
	gatewayForwardingCache.Store(&cachedGatewayForwardingSettings{})
	openAICacheWriteInferenceSettingEpoch.Store(0)
	t.Cleanup(func() {
		gatewayForwardingSF.Forget("gateway_forwarding")
		gatewayForwardingCache.Store(&cachedGatewayForwardingSettings{})
		openAICacheWriteInferenceSettingEpoch.Store(0)
	})
}

func TestOpenAICacheWriteInferenceSwitch_DefaultOffAndClearsState(t *testing.T) {
	resetOpenAICacheWriteInferenceSwitchForTest(t)

	settingService := &SettingService{}
	svc := &OpenAIGatewayService{settingService: settingService}
	account := &Account{
		ID:       77,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
	}
	result := &OpenAIForwardResult{
		RequestID: "req-1",
		Model:     "gpt-6-astra",
		Usage: OpenAIUsage{
			InputTokens:          10000,
			CacheReadInputTokens: 8000,
		},
	}

	// Missing/default runtime state is fail-closed.
	observeOpenAICacheWriteSwitchFixture(svc, context.Background(), account, "gpt-6-astra", "session-a", result)
	require.Empty(t, svc.openaiCacheWriteInferenceTracker.entries)

	// An admin settings refresh makes the opt-in visible immediately.
	settingService.refreshCachedSettings(&SystemSettings{
		OpenAICacheWriteInferenceEnabled: true,
	})
	require.True(t, settingService.IsOpenAICacheWriteInferenceEnabled(context.Background()))
	observeOpenAICacheWriteSwitchFixture(svc, context.Background(), account, "gpt-6-astra", "session-a", result)
	require.Len(t, svc.openaiCacheWriteInferenceTracker.entries, 1)

	// Disabling clears the lineage on the next observation.
	settingService.refreshCachedSettings(&SystemSettings{
		OpenAICacheWriteInferenceEnabled: false,
	})
	result.RequestID = "req-2"
	result.Usage.InputTokens = 11000
	result.Usage.CacheReadInputTokens = 9000
	observeOpenAICacheWriteSwitchFixture(svc, context.Background(), account, "gpt-6-astra", "session-a", result)
	require.Empty(t, svc.openaiCacheWriteInferenceTracker.entries)
}

func TestOpenAICacheWriteInferenceSwitch_OffOnWithoutTrafficDoesNotBridgeLineage(t *testing.T) {
	resetOpenAICacheWriteInferenceSwitchForTest(t)

	settingService := &SettingService{}
	svc := &OpenAIGatewayService{settingService: settingService}
	account := &Account{ID: 78, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	result := &OpenAIForwardResult{
		RequestID: "req-before-off",
		Model:     "gpt-6-astra",
		Usage: OpenAIUsage{
			InputTokens:          10000,
			CacheReadInputTokens: 8000,
		},
	}

	settingService.refreshCachedSettings(&SystemSettings{OpenAICacheWriteInferenceEnabled: true})
	observeOpenAICacheWriteSwitchFixture(svc, context.Background(), account, "gpt-6-astra", "session-b", result)
	require.Len(t, svc.openaiCacheWriteInferenceTracker.entries, 1)
	beforeEpoch := openAICacheWriteInferenceSettingEpoch.Load()

	// No request is made while disabled. Re-enabling still advances the setting
	// epoch twice, so the next observation must not reuse the pre-off lineage.
	settingService.refreshCachedSettings(&SystemSettings{OpenAICacheWriteInferenceEnabled: false})
	settingService.refreshCachedSettings(&SystemSettings{OpenAICacheWriteInferenceEnabled: true})
	require.Greater(t, openAICacheWriteInferenceSettingEpoch.Load(), beforeEpoch)

	result.RequestID = "req-after-reenable"
	result.Usage.InputTokens = 11000
	result.Usage.CacheReadInputTokens = 9000
	observeOpenAICacheWriteSwitchFixture(svc, context.Background(), account, "gpt-6-astra", "session-b", result)

	key := openAICacheWriteTrackerKey(account.ID, "", "session-b", 1)
	entry, ok := svc.openaiCacheWriteInferenceTracker.entries[key]
	require.True(t, ok)
	require.Equal(t, uint64(1), entry.observation.Sequence,
		"the first post-reenable turn must establish a fresh baseline")
	require.WithinDuration(t, time.Now(), entry.observation.ObservedAt, time.Second)
}

func TestOpenAICacheWriteInferenceSwitch_LoadsFromRuntimeSettingsCache(t *testing.T) {
	resetOpenAICacheWriteInferenceSwitchForTest(t)

	repo := &contentModerationTestSettingRepo{values: map[string]string{
		SettingKeyOpenAICacheWriteInferenceEnabled: "true",
	}}
	settingService := &SettingService{settingRepo: repo}

	require.True(t, settingService.IsOpenAICacheWriteInferenceEnabled(context.Background()))
	epochAfterEnable := openAICacheWriteInferenceSettingEpoch.Load()

	// Expire the current true entry without changing its value, then let the
	// runtime cache reload the false value from storage.
	storeGatewayForwardingCache(&cachedGatewayForwardingSettings{
		openAICacheWriteInference: true,
		expiresAt:                 0,
	})
	repo.values[SettingKeyOpenAICacheWriteInferenceEnabled] = "false"

	require.False(t, settingService.IsOpenAICacheWriteInferenceEnabled(context.Background()))
	require.Greater(t, openAICacheWriteInferenceSettingEpoch.Load(), epochAfterEnable)
}

func observeOpenAICacheWriteSwitchFixture(svc *OpenAIGatewayService, ctx context.Context, account *Account, model, identity string, result *OpenAIForwardResult) string {
	body := []byte(`{"model":"gpt-6-astra","input":[{"role":"user","content":"fixture"}]}`)
	result.CacheWritePromptEvidence = buildOpenAICacheWritePromptEvidence(body)
	result.CacheWriteOutputEvidence = buildOpenAICacheWriteOutputEvidence([]byte(`{"output":[]}`))
	request := svc.BeginOpenAICacheWriteObservation(ctx, account, 1, model, identity, body)
	defer svc.CancelOpenAICacheWriteObservation(request)
	return svc.ObserveOpenAICacheWriteTelemetry(ctx, account, model, identity, result, request)
}
