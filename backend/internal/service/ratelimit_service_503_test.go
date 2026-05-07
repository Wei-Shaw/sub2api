//go:build unit

package service

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestRateLimitService_HandleUpstreamError_OpenAI503BurstTempUnschedulable(t *testing.T) {
	repo := &rateLimitAccountRepoStub{}
	service := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	account := &Account{
		ID:       401,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
	}
	body := []byte(`{"error":{"message":"Service temporarily unavailable"}}`)

	require.False(t, service.HandleUpstreamError(context.Background(), account, http.StatusServiceUnavailable, http.Header{}, body))
	require.False(t, service.HandleUpstreamError(context.Background(), account, http.StatusServiceUnavailable, http.Header{}, body))
	require.True(t, service.HandleUpstreamError(context.Background(), account, http.StatusServiceUnavailable, http.Header{}, body))

	require.Equal(t, 1, repo.tempCalls)

	var state TempUnschedState
	require.NoError(t, json.Unmarshal([]byte(repo.lastTempReason), &state))
	require.Equal(t, http.StatusServiceUnavailable, state.StatusCode)
	require.Equal(t, 3, state.HitCount)
	require.Equal(t, openAI503BurstDisableThreshold, state.TriggerCount)
}

func TestRateLimitService_HandleTempUnschedulable_RespectsTriggerCount(t *testing.T) {
	repo := &rateLimitAccountRepoStub{}
	service := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	account := &Account{
		ID:       402,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"temp_unschedulable_enabled": true,
			"temp_unschedulable_rules": []any{
				map[string]any{
					"error_code":       float64(http.StatusServiceUnavailable),
					"keywords":         []any{"temporarily unavailable"},
					"trigger_count":    float64(2),
					"duration_minutes": float64(15),
					"description":      "openai 503 threshold",
				},
			},
		},
	}
	body := []byte(`service temporarily unavailable`)

	require.False(t, service.HandleTempUnschedulable(context.Background(), account, http.StatusServiceUnavailable, body))
	require.Equal(t, 0, repo.tempCalls)

	require.True(t, service.HandleTempUnschedulable(context.Background(), account, http.StatusServiceUnavailable, body))
	require.Equal(t, 1, repo.tempCalls)

	var state TempUnschedState
	require.NoError(t, json.Unmarshal([]byte(repo.lastTempReason), &state))
	require.Equal(t, 2, state.HitCount)
	require.Equal(t, 2, state.TriggerCount)
	require.Equal(t, http.StatusServiceUnavailable, state.StatusCode)
}
