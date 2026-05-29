//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDeepSeekTokenProvider_APIKey(t *testing.T) {
	provider := NewDeepSeekTokenProvider()
	account := &Account{
		ID:       1,
		Platform: PlatformDeepSeek,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key": "sk-deepseek-test-key",
		},
	}

	token, err := provider.GetAccessToken(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, "sk-deepseek-test-key", token)
}

func TestDeepSeekTokenProvider_NilAccount(t *testing.T) {
	provider := NewDeepSeekTokenProvider()

	token, err := provider.GetAccessToken(context.Background(), nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "account is nil")
	require.Empty(t, token)
}

func TestDeepSeekTokenProvider_WrongPlatform(t *testing.T) {
	provider := NewDeepSeekTokenProvider()
	account := &Account{
		ID:       2,
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
	}

	token, err := provider.GetAccessToken(context.Background(), account)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not a deepseek account")
	require.Empty(t, token)
}

func TestDeepSeekTokenProvider_MissingAPIKey(t *testing.T) {
	provider := NewDeepSeekTokenProvider()
	account := &Account{
		ID:          3,
		Platform:    PlatformDeepSeek,
		Type:        AccountTypeAPIKey,
		Credentials: map[string]any{},
	}

	token, err := provider.GetAccessToken(context.Background(), account)
	require.Error(t, err)
	require.Contains(t, err.Error(), "api_key not found in credentials")
	require.Empty(t, token)
}

func TestDeepSeekTokenProvider_OAuthNotImplemented(t *testing.T) {
	provider := NewDeepSeekTokenProvider()
	account := &Account{
		ID:       4,
		Platform: PlatformDeepSeek,
		Type:     AccountTypeOAuth,
	}

	token, err := provider.GetAccessToken(context.Background(), account)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrDeepSeekOAuthNotImplemented)
	require.Empty(t, token)
}
