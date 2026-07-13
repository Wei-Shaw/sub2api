package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSelectCodexModelsAccountWithExclusions_OnlySelectsOpenAIOAuth(t *testing.T) {
	accounts := []Account{
		{
			ID:          1,
			Platform:    PlatformOpenAI,
			Type:        AccountTypeAPIKey,
			Status:      StatusActive,
			Schedulable: true,
			Priority:    0,
			Credentials: map[string]any{"api_key": "test-api-key"},
		},
		{
			ID:          2,
			Platform:    PlatformOpenAI,
			Type:        AccountTypeOAuth,
			Status:      StatusActive,
			Schedulable: true,
			Priority:    1,
			Credentials: map[string]any{"access_token": "test-access-token"},
		},
	}
	svc := &OpenAIGatewayService{accountRepo: schedulerTestOpenAIAccountRepo{accounts: accounts}}

	account, err := svc.SelectCodexModelsAccountWithExclusions(context.Background(), nil, nil)

	require.NoError(t, err)
	require.Equal(t, int64(2), account.ID)
}

func TestSelectCodexModelsAccountWithExclusions_HonorsExclusions(t *testing.T) {
	accounts := []Account{
		{
			ID:          1,
			Platform:    PlatformOpenAI,
			Type:        AccountTypeOAuth,
			Status:      StatusActive,
			Schedulable: true,
			Priority:    0,
			Credentials: map[string]any{"access_token": "token-1"},
		},
		{
			ID:          2,
			Platform:    PlatformOpenAI,
			Type:        AccountTypeOAuth,
			Status:      StatusActive,
			Schedulable: true,
			Priority:    1,
			Credentials: map[string]any{"access_token": "token-2"},
		},
	}
	svc := &OpenAIGatewayService{accountRepo: schedulerTestOpenAIAccountRepo{accounts: accounts}}

	account, err := svc.SelectCodexModelsAccountWithExclusions(
		context.Background(),
		nil,
		map[int64]struct{}{1: {}},
	)

	require.NoError(t, err)
	require.Equal(t, int64(2), account.ID)
}
