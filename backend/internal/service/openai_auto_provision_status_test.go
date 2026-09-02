//go:build unit

package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestOpenAIAutoProvisionRuntimeStatusTracksRequestAndCallback(t *testing.T) {
	settingsRepo := &openAIAutoProvisionFlowSettingRepo{values: map[string]string{
		SettingKeyOpenAIAutoProvisionEnabled:         "true",
		SettingKeyOpenAIAutoProvisionTarget:          "3",
		SettingKeyOpenAIAutoProvisionIntervalSeconds: "15",
		SettingKeyOpenAIAutoProvisionTurbURL:         "http://turb.test",
		SettingKeyOpenAIAutoProvisionTurbAuthCode:    "fake-auth-code",
		SettingKeyOpenAIAutoProvisionCallbackURL:     "http://sub2api.test/api/v1/integrations/openai/auto-provision/callback",
		SettingKeyOpenAIAutoProvisionCallbackSecret:  "fake-callback-secret",
		SettingKeyOpenAIAutoProvisionWorkers:         "2",
	}}
	accounts := &openAIAutoProvisionFlowAccounts{accounts: map[int64]Account{
		1: {
			ID:          1,
			Platform:    PlatformOpenAI,
			Type:        AccountTypeOAuth,
			Status:      StatusActive,
			Schedulable: true,
			Credentials: map[string]any{"email": "healthy@example.com"},
		},
	}}
	client := &openAIAutoProvisionFlowClient{}
	service := NewOpenAIAutoProvisionService(
		accounts,
		accounts,
		NewSettingService(settingsRepo, &config.Config{}),
		settingsRepo,
		nil,
		client,
		nil,
		nil,
	)
	service.SetOpenAIProvisionDemandReader(openAIAutoProvisionFlowDemandReader{
		demand: OpenAIProvisionDemand{ActiveUsers: 3, Requests: 30, Tokens: 900_000, CapacityDeniedUsers: 3, CapacityDeniedRequests: 3},
	})
	ctx := context.Background()

	require.NoError(t, service.RunOnce(ctx))
	status, err := service.GetRuntimeStatus(ctx)
	require.NoError(t, err)
	require.Equal(t, "waiting_for_provision_callback", status.Phase)
	require.Equal(t, 1, status.HealthyAccountCount)
	require.Equal(t, 3, status.Target)
	require.Equal(t, 1, status.PendingProvisionCount)
	require.Equal(t, 1, status.LastProvisionRequestedCount)
	require.NotNil(t, status.LastCheckCompletedAt)
	require.NotNil(t, status.LastProvisionRequestedAt)

	stale := time.Now().UTC().Add(-autoProvisionDispatchStaleTTL - time.Second)
	require.NoError(t, settingsRepo.Set(ctx, autoProvisionStateKey, mustMarshalAutoProvisionState(t, &autoProvisionState{
		Version:   autoProvisionStateVersion,
		Provision: &autoProvisionPending{RequestID: client.provision[0].RequestID, RequestedCount: 1, CreatedAt: stale},
	})))
	require.NoError(t, service.ResetProvisioningStatus(ctx))
	status, err = service.GetRuntimeStatus(ctx)
	require.NoError(t, err)
	require.Equal(t, "idle", status.Phase)
	require.Equal(t, 0, status.PendingProvisionCount)
	require.False(t, status.ProvisionRetryRequested)

	// The next existing scheduler cycle must be able to dispatch again.
	require.NoError(t, service.RunOnce(ctx))
	require.Len(t, client.provision, 2)
	require.NotEqual(t, client.provision[0].RequestID, client.provision[1].RequestID)

	// A callback for the retired Turb request is still accepted without
	// consuming the replacement request.
	lateReplay, lateErr := service.HandleProvisionCallback(ctx, OpenAIAutoProvisionCallback{
		RequestID:      client.provision[0].RequestID,
		EventID:        client.provision[0].RequestID + ":registration:completed",
		Kind:           "registration",
		Status:         "completed",
		RequestedCount: 1,
		SucceededCount: 1,
	})
	require.NoError(t, lateErr)
	require.False(t, lateReplay)
	status, err = service.GetRuntimeStatus(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, status.PendingProvisionCount)

	replay, err := service.HandleProvisionCallback(ctx, OpenAIAutoProvisionCallback{
		RequestID:      client.provision[1].RequestID,
		EventID:        client.provision[1].RequestID + ":registration:completed",
		Kind:           "registration",
		Status:         "completed",
		RequestedCount: 1,
		SucceededCount: 1,
		FailedCount:    1,
		PendingCount:   0,
	})
	require.NoError(t, err)
	require.False(t, replay)

	status, err = service.GetRuntimeStatus(ctx)
	require.NoError(t, err)
	require.Equal(t, "idle", status.Phase)
	require.Equal(t, 0, status.PendingProvisionCount)
	require.Equal(t, "registration", status.LastCallbackKind)
	require.Equal(t, "completed", status.LastCallbackStatus)
	require.Equal(t, 1, status.LastCallbackSucceededCount)
	require.Equal(t, 1, status.LastCallbackFailedCount)
	require.NotNil(t, status.LastCallbackAt)
}

func TestOpenAIAutoProvisionResetRejectsActiveDispatchAndRecoversStaleDispatch(t *testing.T) {
	service, settingsRepo := newOpenAIAutoProvisionStatusTestService(t)
	ctx := context.Background()
	now := time.Now().UTC()
	require.NoError(t, settingsRepo.Set(ctx, autoProvisionStateKey, mustMarshalAutoProvisionState(t, &autoProvisionState{
		Version:                     autoProvisionStateVersion,
		Provision:                   &autoProvisionPending{RequestID: "active-request", RequestedCount: 2, CreatedAt: now},
		ProvisionDispatchInProgress: true,
		ProvisionDispatchStartedAt:  &now,
	})))

	require.Error(t, service.ResetProvisioningStatus(ctx))

	stale := now.Add(-autoProvisionDispatchStaleTTL - time.Second)
	require.NoError(t, settingsRepo.Set(ctx, autoProvisionStateKey, mustMarshalAutoProvisionState(t, &autoProvisionState{
		Version:                     autoProvisionStateVersion,
		Provision:                   &autoProvisionPending{RequestID: "stale-request", RequestedCount: 2, CreatedAt: stale},
		ProvisionDispatchInProgress: true,
		ProvisionDispatchStartedAt:  &stale,
	})))
	require.NoError(t, service.ResetProvisioningStatus(ctx))
	status, err := service.GetRuntimeStatus(ctx)
	require.NoError(t, err)
	require.Equal(t, OpenAIAutoProvisionPhaseIdle, status.Phase)
	require.False(t, status.ProvisionResettable)
	require.False(t, status.ProvisionDispatchStale)
}

func TestOpenAIAutoProvisionRuntimeStatusReportsConfigurationError(t *testing.T) {
	service, settingsRepo := newOpenAIAutoProvisionStatusTestService(t)
	ctx := context.Background()
	require.NoError(t, settingsRepo.Set(ctx, SettingKeyOpenAIAutoProvisionCallbackSecret, ""))

	err := service.RunOnce(ctx)
	require.Error(t, err)
	status, statusErr := service.GetRuntimeStatus(ctx)
	require.NoError(t, statusErr)
	require.Equal(t, OpenAIAutoProvisionPhaseError, status.Phase)
	require.Contains(t, status.LastError, "callback secret")
}

func newOpenAIAutoProvisionStatusTestService(t *testing.T) (*OpenAIAutoProvisionService, *openAIAutoProvisionFlowSettingRepo) {
	t.Helper()
	settingsRepo := &openAIAutoProvisionFlowSettingRepo{values: map[string]string{
		SettingKeyOpenAIAutoProvisionEnabled:         "true",
		SettingKeyOpenAIAutoProvisionTarget:          "3",
		SettingKeyOpenAIAutoProvisionIntervalSeconds: "15",
		SettingKeyOpenAIAutoProvisionTurbURL:         "http://turb.test",
		SettingKeyOpenAIAutoProvisionTurbAuthCode:    "fake-auth-code",
		SettingKeyOpenAIAutoProvisionCallbackURL:     "http://sub2api.test/api/v1/integrations/openai/auto-provision/callback",
		SettingKeyOpenAIAutoProvisionCallbackSecret:  "fake-callback-secret",
		SettingKeyOpenAIAutoProvisionWorkers:         "2",
	}}
	accounts := &openAIAutoProvisionFlowAccounts{accounts: map[int64]Account{
		1: {
			ID:          1,
			Platform:    PlatformOpenAI,
			Type:        AccountTypeOAuth,
			Status:      StatusActive,
			Schedulable: true,
			Credentials: map[string]any{"email": "healthy@example.com"},
		},
	}}
	service := NewOpenAIAutoProvisionService(
		accounts,
		accounts,
		NewSettingService(settingsRepo, &config.Config{}),
		settingsRepo,
		nil,
		&openAIAutoProvisionFlowClient{},
		nil,
		nil,
	)
	service.SetOpenAIProvisionDemandReader(openAIAutoProvisionFlowDemandReader{
		demand: OpenAIProvisionDemand{ActiveUsers: 3, Requests: 30, Tokens: 900_000, CapacityDeniedUsers: 3, CapacityDeniedRequests: 3},
	})
	return service, settingsRepo
}

func mustMarshalAutoProvisionState(t *testing.T, state *autoProvisionState) string {
	t.Helper()
	encoded, err := json.Marshal(normalizeAutoProvisionState(state))
	require.NoError(t, err)
	return string(encoded)
}
