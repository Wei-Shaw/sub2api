//go:build unit

package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestResolveGrokStreamIdleTimeout(t *testing.T) {
	require.Equal(t, 90*time.Second, resolveGrokStreamIdleTimeout(90))
	require.Equal(t, defaultGrokStreamIdleTimeout, resolveGrokStreamIdleTimeout(0))
	require.Equal(t, defaultGrokStreamIdleTimeout, resolveGrokStreamIdleTimeout(-1))
}

func TestGrokStreamIdleFailoverError(t *testing.T) {
	account := &Account{ID: 1, Platform: PlatformGrok, Type: AccountTypeOAuth}
	err := grokStreamIdleFailoverError(account, 180*time.Second)
	require.NotNil(t, err)
	require.Equal(t, 502, err.StatusCode)
	require.True(t, err.SafeToFailoverAfterWrite)
	require.True(t, err.RetryableOnSameAccount)
	require.True(t, err.RequestScopedTransient)
	require.Equal(t, 1, err.SameAccountRetryMax)
	require.Contains(t, string(err.ResponseBody), "empty_upstream")
	require.WithinDuration(t, time.Now().Add(180*time.Second), err.SameAccountRetryDeadline, 2*time.Second)
}

func TestGrokStreamIdleFailoverErrorRequiresGrokAccount(t *testing.T) {
	openAI := &Account{ID: 2, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	err := grokStreamIdleFailoverError(openAI, time.Second)
	require.False(t, err.RetryableOnSameAccount)
	require.True(t, err.RequestScopedTransient)
}
