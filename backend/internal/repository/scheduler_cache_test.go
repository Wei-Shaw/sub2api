package repository

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestFilterSchedulerCredentialsKeepsSubscriptionPlanType(t *testing.T) {
	filtered := filterSchedulerCredentials(map[string]any{
		"plan_type":     "plus",
		"access_token":  "secret-access-token",
		"refresh_token": "secret-refresh-token",
	})

	require.Equal(t, "plus", filtered["plan_type"])
	require.NotContains(t, filtered, "access_token")
	require.NotContains(t, filtered, "refresh_token")
}

func TestSchedulerMetadataAccountKeepsOpenAISubscriptionIdentity(t *testing.T) {
	account := service.Account{
		ID:       24,
		Platform: service.PlatformOpenAI,
		Type:     service.AccountTypeOAuth,
		Credentials: map[string]any{
			"plan_type":    "plus",
			"access_token": "secret-access-token",
		},
	}

	metadata := buildSchedulerMetadataAccount(account)

	require.True(t, metadata.IsOpenAIChatGPTSubscription())
	require.Empty(t, metadata.GetCredential("access_token"))
}

func TestSchedulerMetadataAccountKeepsGrokRecoveryAndPaidIdentity(t *testing.T) {
	t.Run("pending free account stays unschedulable after reset expiry", func(t *testing.T) {
		account := service.Account{
			ID:          25,
			Platform:    service.PlatformGrok,
			Type:        service.AccountTypeOAuth,
			Status:      service.StatusActive,
			Schedulable: true,
			Extra: map[string]any{
				service.GrokFreeRecoveryPendingExtraKey: true,
			},
		}

		metadata := buildSchedulerMetadataAccount(account)

		require.True(t, metadata.IsGrokFreeRecoveryPending())
		require.False(t, metadata.IsSchedulable())
	})

	t.Run("paid subscription evidence survives metadata filtering", func(t *testing.T) {
		account := service.Account{
			ID:       26,
			Platform: service.PlatformGrok,
			Type:     service.AccountTypeOAuth,
			Credentials: map[string]any{
				"subscription_tier": "SuperGrok",
				"access_token":      "secret-access-token",
			},
		}

		metadata := buildSchedulerMetadataAccount(account)

		require.False(t, metadata.IsGrokFreeOrUnknownOAuth())
		require.Equal(t, "SuperGrok", metadata.GetCredential("subscription_tier"))
		require.Empty(t, metadata.GetCredential("access_token"))
	})
}
