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

func TestRateLimitService_HandleUpstreamError_402_PoolMode_SetsTempUnschedulable(t *testing.T) {
	repo := &rateLimitAccountRepoStub{}
	blocker := &runtimeBlockRecorder{}
	service := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	service.SetAccountRuntimeBlocker(blocker)

	account := &Account{
		ID:          4021,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Credentials: map[string]any{
			"pool_mode": true,
		},
	}
	require.True(t, account.IsSchedulable())

	shouldDisable := service.HandleUpstreamError(
		context.Background(),
		account,
		http.StatusPaymentRequired,
		http.Header{},
		[]byte(`{"error":{"message":"You have exceeded your current quota, please check your plan and billing details."}}`),
	)

	require.True(t, shouldDisable)
	require.Equal(t, 0, repo.setErrorCalls)
	require.Equal(t, 1, repo.tempCalls)
	require.NotNil(t, account.TempUnschedulableUntil)
	require.True(t, account.TempUnschedulableUntil.After(time.Now().Add(25*time.Minute)))
	require.Contains(t, repo.lastTempReason, "payment_required")
	require.False(t, account.IsSchedulable(), "cooled down account must be unschedulable")

	require.Len(t, blocker.accounts, 1)
	require.Equal(t, account.ID, blocker.accounts[0].ID)
	require.Equal(t, "payment_required_402", blocker.reasons[0])
	require.True(t, blocker.until[0].After(time.Now()))
}

func TestRateLimitService_CheckErrorPolicy_402_PoolMode_ReturnsTempUnscheduled(t *testing.T) {
	repo := &rateLimitAccountRepoStub{}
	service := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)

	account := &Account{
		ID:          4022,
		Platform:    PlatformAnthropic,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Credentials: map[string]any{
			"pool_mode": true,
		},
	}

	result := service.CheckErrorPolicy(
		context.Background(),
		account,
		http.StatusPaymentRequired,
		[]byte(`{"type":"error","error":{"type":"billing_error","message":"Your credit balance is too low."}}`),
	)

	require.Equal(t, ErrorPolicyTempUnscheduled, result)
	require.Equal(t, 1, repo.tempCalls)
	require.NotNil(t, account.TempUnschedulableUntil)
	require.False(t, account.IsSchedulable())
}

func TestRateLimitService_HandleUpstreamError_402_CustomRulesTakePrecedence(t *testing.T) {
	repo := &rateLimitAccountRepoStub{}
	service := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)

	account := &Account{
		ID:          4023,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Credentials: map[string]any{
			"pool_mode":                  true,
			"temp_unschedulable_enabled": true,
			"temp_unschedulable_rules": []any{
				map[string]any{
					"error_code":       float64(402),
					"keywords":         []any{"custom_low_balance"},
					"duration_minutes": float64(15),
				},
			},
		},
	}

	shouldDisable := service.HandleUpstreamError(
		context.Background(),
		account,
		http.StatusPaymentRequired,
		http.Header{},
		[]byte(`{"error":{"message":"custom_low_balance hit"}}`),
	)

	require.True(t, shouldDisable)
	require.Equal(t, 1, repo.tempCalls)
	require.Contains(t, repo.lastTempReason, `"rule_index":0`)
	require.Contains(t, repo.lastTempReason, "custom_low_balance")
}

func TestRateLimitService_HandleUpstreamError_402_NonPoolMode_WithoutConfig_KeepsSetError(t *testing.T) {
	repo := &rateLimitAccountRepoStub{}
	service := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)

	account := &Account{
		ID:          4024,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Credentials: map[string]any{},
	}

	shouldDisable := service.HandleUpstreamError(
		context.Background(),
		account,
		http.StatusPaymentRequired,
		http.Header{},
		[]byte(`{"error":{"message":"insufficient funds"}}`),
	)

	require.True(t, shouldDisable)
	require.Equal(t, 1, repo.setErrorCalls)
	require.Equal(t, 0, repo.tempCalls)
	require.Contains(t, repo.lastErrorMsg, "Payment required (402)")
}

func TestRateLimitService_HandleUpstreamError_402_NonPoolMode_WithConfig_SetsTempUnschedulable(t *testing.T) {
	repo := &rateLimitAccountRepoStub{}
	cfg := &config.Config{
		RateLimit: config.RateLimitConfig{
			PaymentRequired402CooldownMinutes: 45,
		},
	}
	service := NewRateLimitService(repo, nil, cfg, nil, nil)

	account := &Account{
		ID:          4025,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Credentials: map[string]any{},
	}

	shouldDisable := service.HandleUpstreamError(
		context.Background(),
		account,
		http.StatusPaymentRequired,
		http.Header{},
		[]byte(`{"error":{"message":"insufficient balance"}}`),
	)

	require.True(t, shouldDisable)
	require.Equal(t, 0, repo.setErrorCalls)
	require.Equal(t, 1, repo.tempCalls)
	require.True(t, account.TempUnschedulableUntil.After(time.Now().Add(40*time.Minute)))
	require.False(t, account.IsSchedulable())
}

func TestRateLimitService_HandleUpstreamError_402_OpenAIDeactivatedWorkspace_AlwaysDisables(t *testing.T) {
	repo := &rateLimitAccountRepoStub{}
	service := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)

	account := &Account{
		ID:          4026,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Credentials: map[string]any{
			"pool_mode": true,
		},
	}

	shouldDisable := service.HandleUpstreamError(
		context.Background(),
		account,
		http.StatusPaymentRequired,
		http.Header{},
		[]byte(`{"detail":{"code":"deactivated_workspace","message":"This workspace has been deactivated."}}`),
	)

	require.True(t, shouldDisable)
	require.Equal(t, 1, repo.setErrorCalls)
	require.Equal(t, 0, repo.tempCalls)
	require.Contains(t, repo.lastErrorMsg, "Workspace deactivated (402)")
}
