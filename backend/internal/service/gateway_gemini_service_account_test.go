package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type mockGeminiTokenCache struct{}

func (m *mockGeminiTokenCache) GetAccessToken(ctx context.Context, cacheKey string) (string, error) {
	return "mock-gemini-token", nil
}

func (m *mockGeminiTokenCache) SetAccessToken(ctx context.Context, cacheKey string, accessToken string, ttl any) error {
	return nil
}

func (m *mockGeminiTokenCache) AcquireRefreshLock(ctx context.Context, cacheKey string, ttl any) (bool, error) {
	return true, nil
}

func (m *mockGeminiTokenCache) ReleaseRefreshLock(ctx context.Context, cacheKey string) error {
	return nil
}

func TestGatewayService_GetAccessToken_GeminiServiceAccount(t *testing.T) {
	geminiTP := NewGeminiTokenProvider(nil, &mockGeminiTokenCache{}, nil)

	svc := &GatewayService{
		geminiTokenProvider: geminiTP,
	}

	account := &Account{
		ID:       39,
		Platform: PlatformGemini,
		Type:     AccountTypeServiceAccount,
		Credentials: map[string]any{
			"project_id": "test-project",
			"service_account_json": `{
				"type": "service_account",
				"project_id": "test-project",
				"private_key_id": "key-id",
				"private_key": "-----BEGIN PRIVATE KEY-----\nMIIEvgIBADANBgkqhkiG9w0BAQEFAASCBKgwggSkAgEAAoIBAQC3\n-----END PRIVATE KEY-----\n",
				"client_email": "test@test-project.iam.gserviceaccount.com"
			}`,
		},
	}

	token, tokenType, err := svc.GetAccessToken(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, "mock-gemini-token", token)
	require.Equal(t, "service_account", tokenType)
}

func TestGatewayService_GetAccessToken_GeminiOAuth(t *testing.T) {
	geminiTP := NewGeminiTokenProvider(nil, &mockGeminiTokenCache{}, nil)

	svc := &GatewayService{
		geminiTokenProvider: geminiTP,
	}

	account := &Account{
		ID:       40,
		Platform: PlatformGemini,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": "fallback-token",
		},
	}

	token, tokenType, err := svc.GetAccessToken(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, "mock-gemini-token", token)
	require.Equal(t, "oauth", tokenType)
}
