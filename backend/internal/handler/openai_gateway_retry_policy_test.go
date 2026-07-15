package handler

import (
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestOpenAIGatewaySameAccountRetryDelay_ExplicitProfileOverridesAccountPoolConfig(t *testing.T) {
	profile := []time.Duration{200 * time.Millisecond, 500 * time.Millisecond, time.Second}
	failoverErr := &service.UpstreamFailoverError{
		RetryableOnSameAccount:   true,
		SameAccountRetryBackoffs: profile,
	}

	for _, configuredRetries := range []int{0, 1, 10} {
		t.Run(fmt.Sprintf("pool_retry_count_%d", configuredRetries), func(t *testing.T) {
			account := &service.Account{
				Type:     service.AccountTypeAPIKey,
				Platform: service.PlatformOpenAI,
				Credentials: map[string]any{
					"pool_mode":             true,
					"pool_mode_retry_count": configuredRetries,
				},
			}

			for completedRetries, want := range profile {
				delay, ok := openAIGatewaySameAccountRetryDelay(account, failoverErr, completedRetries)
				require.True(t, ok)
				require.Equal(t, want, delay)
			}
			_, ok := openAIGatewaySameAccountRetryDelay(account, failoverErr, len(profile))
			require.False(t, ok)
		})
	}
}

func TestOpenAIGatewaySameAccountRetryDelay_EmptyProfileUsesGenericAccountPolicy(t *testing.T) {
	account := &service.Account{
		Type:     service.AccountTypeAPIKey,
		Platform: service.PlatformOpenAI,
		Credentials: map[string]any{
			"pool_mode":             true,
			"pool_mode_retry_count": 2,
		},
	}
	failoverErr := &service.UpstreamFailoverError{
		RetryableOnSameAccount:   true,
		SameAccountRetryBackoffs: []time.Duration{},
	}

	require.Equal(t, 2, account.GetPoolModeRetryCount())
	for completedRetries := 0; completedRetries < account.GetPoolModeRetryCount(); completedRetries++ {
		delay, ok := openAIGatewaySameAccountRetryDelay(account, failoverErr, completedRetries)
		require.True(t, ok)
		require.Equal(t, sameAccountRetryDelay, delay)
	}
	_, ok := openAIGatewaySameAccountRetryDelay(account, failoverErr, account.GetPoolModeRetryCount())
	require.False(t, ok)
}
