package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDiagnoseOpenAIModelAvailability_AllSupportingAccountsModelNotFoundLimited(t *testing.T) {
	model := "gpt-5.6-luna"
	resetAt := time.Now().Add(10 * time.Minute).UTC().Format(time.RFC3339)
	accounts := []Account{
		openAIAccountWithModelRateLimit(1, model, resetAt, upstreamModelNotFoundReason),
		openAIAccountWithModelRateLimit(2, model, resetAt, upstreamModelNotFoundReason),
	}

	got := diagnoseOpenAIModelAvailability(context.Background(), accounts, model)

	require.True(t, got.HasAccountsInPool)
	require.True(t, got.HasModelSupport)
	require.True(t, got.AllSupportingAccountsModelNotFoundLimited)
}

func TestDiagnoseOpenAIModelAvailability_OneSupportingAccountAvailable(t *testing.T) {
	model := "gpt-5.6-luna"
	resetAt := time.Now().Add(10 * time.Minute).UTC().Format(time.RFC3339)
	accounts := []Account{
		openAIAccountWithModelRateLimit(1, model, resetAt, upstreamModelNotFoundReason),
		{ID: 2, Platform: PlatformOpenAI},
	}

	got := diagnoseOpenAIModelAvailability(context.Background(), accounts, model)

	require.True(t, got.HasModelSupport)
	require.False(t, got.AllSupportingAccountsModelNotFoundLimited)
}

func TestDiagnoseOpenAIModelAvailability_DifferentCooldownReasonStaysTransient(t *testing.T) {
	model := "gpt-5.6-luna"
	resetAt := time.Now().Add(10 * time.Minute).UTC().Format(time.RFC3339)
	accounts := []Account{
		openAIAccountWithModelRateLimit(1, model, resetAt, "upstream_rate_limit"),
	}

	got := diagnoseOpenAIModelAvailability(context.Background(), accounts, model)

	require.True(t, got.HasModelSupport)
	require.False(t, got.AllSupportingAccountsModelNotFoundLimited)
}

func TestDiagnoseOpenAIModelAvailability_ExpiredModelNotFoundCooldownStaysTransient(t *testing.T) {
	model := "gpt-5.6-luna"
	resetAt := time.Now().Add(-time.Minute).UTC().Format(time.RFC3339)
	accounts := []Account{
		openAIAccountWithModelRateLimit(1, model, resetAt, upstreamModelNotFoundReason),
	}

	got := diagnoseOpenAIModelAvailability(context.Background(), accounts, model)

	require.True(t, got.HasModelSupport)
	require.False(t, got.AllSupportingAccountsModelNotFoundLimited)
}

func openAIAccountWithModelRateLimit(id int64, model, resetAt, reason string) Account {
	return Account{
		ID:       id,
		Platform: PlatformOpenAI,
		Extra: map[string]any{
			modelRateLimitsKey: map[string]any{
				model: map[string]any{
					"rate_limit_reset_at": resetAt,
					"reason":              reason,
				},
			},
		},
	}
}
