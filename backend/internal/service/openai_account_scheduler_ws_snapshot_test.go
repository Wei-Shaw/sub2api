//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestOpenAIGatewayService_SelectAccountWithScheduler_UsesWSPassthroughSnapshotFlags(t *testing.T) {
	ctx := context.Background()
	groupID := int64(10105)
	account := &Account{
		ID:          35001,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 10,
		GroupIDs:    []int64{groupID},
		Extra: map[string]any{
			"openai_oauth_responses_websockets_v2_mode": OpenAIWSIngressModePassthrough,
		},
	}

	snapshotCache := &openAISnapshotCacheStub{
		snapshotAccounts: []*Account{account},
		accountsByID:     map[int64]*Account{account.ID: account},
	}
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.OAuthEnabled = true
	cfg.Gateway.OpenAIWS.APIKeyEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
	cfg.Gateway.OpenAIWS.ModeRouterV2Enabled = true
	cfg.Gateway.OpenAIWS.IngressModeDefault = OpenAIWSIngressModeCtxPool

	svc := &OpenAIGatewayService{
		accountRepo:        schedulerTestOpenAIAccountRepo{accounts: []Account{*account}},
		cache:              &schedulerTestGatewayCache{},
		cfg:                cfg,
		rateLimitService:   newOpenAIAdvancedSchedulerRateLimitService("true"),
		schedulerSnapshot:  &SchedulerSnapshotService{cache: snapshotCache},
		concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{}),
	}

	selection, decision, err := svc.SelectAccountWithScheduler(
		ctx,
		&groupID,
		"",
		"session_hash_ws_passthrough",
		"gpt-5.1",
		nil,
		OpenAIUpstreamTransportResponsesWebsocketV2,
		false,
	)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	require.Equal(t, account.ID, selection.Account.ID)
	require.Equal(t, openAIAccountScheduleLayerLoadBalance, decision.Layer)
}

func TestOpenAIGatewayService_ShadowSchedulingInheritsCurrentParentProfile(t *testing.T) {
	ctx := context.Background()
	parentID := int64(36000)
	shadow := &Account{
		ID:              36001,
		Platform:        PlatformOpenAI,
		Type:            AccountTypeOAuth,
		Status:          StatusActive,
		Schedulable:     true,
		Concurrency:     4,
		ParentAccountID: &parentID,
		QuotaDimension:  QuotaDimensionSpark,
		Credentials: map[string]any{
			"model_mapping": map[string]any{"gpt-5.3-codex-spark": "gpt-5.3-codex-spark"},
		},
		Extra: map[string]any{
			openAILongContextBillingEnabledKey:          false,
			"openai_device_id":                          "shadow-device",
			codexFingerprintSeedExtraKey:                "shadow-seed",
			"codex_fingerprint_mode":                    "device",
			"openai_oauth_responses_websockets_v2_mode": OpenAIWSIngressModeOff,
			"codex_7d_used_percent":                     25.0,
		},
	}
	parent := &Account{
		ID:          parentID,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: false,
		Credentials: map[string]any{"access_token": "parent-token-v1"},
		Extra: map[string]any{
			openAILongContextBillingEnabledKey:          true,
			"openai_device_id":                          "parent-device-v1",
			codexFingerprintSeedExtraKey:                "parent-seed-v1",
			"codex_fingerprint_mode":                    "session",
			"openai_oauth_responses_websockets_v2_mode": OpenAIWSIngressModePassthrough,
		},
	}
	snapshotCache := &openAISnapshotCacheStub{
		snapshotAccounts: []*Account{shadow},
		accountsByID: map[int64]*Account{
			shadow.ID: shadow,
			parent.ID: parent,
		},
	}
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.OAuthEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
	cfg.Gateway.OpenAIWS.ModeRouterV2Enabled = true
	cfg.Gateway.OpenAIWS.IngressModeDefault = OpenAIWSIngressModeCtxPool
	svc := &OpenAIGatewayService{
		cfg:                cfg,
		accountRepo:        schedulerTestOpenAIAccountRepo{accounts: []Account{*shadow, *parent}},
		schedulerSnapshot:  &SchedulerSnapshotService{cache: snapshotCache},
		openaiWSStateStore: NewOpenAIWSStateStore(nil),
	}

	accounts, err := svc.listSchedulableAccounts(ctx, nil, PlatformOpenAI)
	require.NoError(t, err)
	require.Len(t, accounts, 1)
	require.Equal(t, shadow.ID, accounts[0].ID)
	require.Equal(t, QuotaDimensionSpark, accounts[0].QuotaDimension)
	require.Equal(t, 25.0, accounts[0].Extra["codex_7d_used_percent"])
	require.True(t, accounts[0].IsOpenAILongContextBillingEnabled())
	require.Equal(t, "parent-device-v1", accounts[0].GetOpenAIDeviceID())
	require.Equal(t, "parent-seed-v1", accounts[0].Extra[codexFingerprintSeedExtraKey])
	require.Equal(t, "session", accounts[0].GetExtraString("codex_fingerprint_mode"))
	require.True(t, svc.isOpenAIAccountTransportCompatible(&accounts[0], OpenAIUpstreamTransportResponsesWebsocketV2))

	require.NoError(t, svc.openaiWSStateStore.BindResponseAccount(ctx, 0, "resp_shadow", shadow.ID, time.Hour))
	selection, err := svc.SelectAccountByPreviousResponseID(
		ctx, nil, "resp_shadow", "gpt-5.3-codex-spark", nil, false,
	)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.Equal(t, shadow.ID, selection.Account.ID)
	require.Equal(t, "parent-token-v1", selection.Account.GetCredential("access_token"))
	require.Equal(t, "parent-device-v1", selection.Account.GetOpenAIDeviceID())

	updatedParent := *parent
	updatedParent.Credentials = map[string]any{"access_token": "parent-token-v2"}
	updatedParent.Extra = map[string]any{
		openAILongContextBillingEnabledKey:          false,
		"openai_device_id":                          "parent-device-v2",
		codexFingerprintSeedExtraKey:                "parent-seed-v2",
		"codex_fingerprint_mode":                    "full",
		"openai_oauth_responses_websockets_v2_mode": OpenAIWSIngressModeOff,
	}
	snapshotCache.accountsByID[parentID] = &updatedParent

	hydrated, err := svc.hydrateSelectedAccount(ctx, &accounts[0])
	require.NoError(t, err)
	require.Equal(t, "parent-token-v2", hydrated.GetCredential("access_token"))
	require.Equal(t, "parent-device-v2", hydrated.GetOpenAIDeviceID())
	require.Equal(t, "parent-seed-v2", hydrated.Extra[codexFingerprintSeedExtraKey])
	require.False(t, hydrated.IsOpenAILongContextBillingEnabled())
	require.Equal(t, "full", hydrated.GetExtraString("codex_fingerprint_mode"))
	require.False(t, svc.isOpenAIAccountTransportCompatible(hydrated, OpenAIUpstreamTransportResponsesWebsocketV2))
	require.Equal(t, "shadow-device", shadow.GetOpenAIDeviceID())
	require.Equal(t, "shadow-seed", shadow.Extra[codexFingerprintSeedExtraKey])
	require.Equal(t, OpenAIWSIngressModeOff, shadow.ResolveOpenAIResponsesWebSocketV2Mode(OpenAIWSIngressModeCtxPool))
}
