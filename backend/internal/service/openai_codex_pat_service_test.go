package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOpenAIOAuthService_ValidateCodexPersonalAccessToken(t *testing.T) {
	var gotAuthorization string
	var gotOriginator string
	var gotUserAgent string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuthorization = r.Header.Get("authorization")
		gotOriginator = r.Header.Get("originator")
		gotUserAgent = r.Header.Get("user-agent")
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{
			"email":"user@example.com",
			"chatgpt_user_id":"user-123",
			"chatgpt_account_id":"acct-123",
			"chatgpt_plan_type":"plus",
			"chatgpt_account_is_fedramp":true
		}`))
	}))
	defer server.Close()

	originalURL := openAICodexPATWhoamiURL
	openAICodexPATWhoamiURL = server.URL
	defer func() { openAICodexPATWhoamiURL = originalURL }()

	svc := NewOpenAIOAuthService(nil, nil)
	defer svc.Stop()

	info, err := svc.ValidateCodexPersonalAccessToken(context.Background(), " at-test-token ", "")
	require.NoError(t, err)
	require.Equal(t, "Bearer at-test-token", gotAuthorization)
	require.Equal(t, "codex_cli_rs", gotOriginator)
	require.Equal(t, codexCLIUserAgent, gotUserAgent)
	require.Equal(t, OpenAIAuthModePersonalAccessToken, info.AuthMode)
	require.Equal(t, "user@example.com", info.Email)
	require.Equal(t, "user-123", info.ChatGPTUserID)
	require.Equal(t, "acct-123", info.ChatGPTAccountID)
	require.Equal(t, "plus", info.PlanType)
	require.True(t, info.ChatGPTAccountFedRAMP)
	require.Zero(t, info.ExpiresAt)
	require.Empty(t, info.RefreshToken)
}

func TestOpenAIOAuthService_ValidateCodexPersonalAccessTokenRequiresATPrefix(t *testing.T) {
	svc := NewOpenAIOAuthService(nil, nil)
	defer svc.Stop()

	_, err := svc.ValidateCodexPersonalAccessToken(context.Background(), "eyJ.jwt", "")
	require.Error(t, err)
	require.Contains(t, err.Error(), "at-")
}

func TestOpenAIOAuthService_BuildAccountCredentialsForPAT(t *testing.T) {
	svc := NewOpenAIOAuthService(nil, nil)
	defer svc.Stop()

	credentials := svc.BuildAccountCredentials(&OpenAITokenInfo{
		AccessToken:           "at-test-token",
		AuthMode:              OpenAIAuthModePersonalAccessToken,
		Email:                 "user@example.com",
		ChatGPTAccountID:      "acct-123",
		ChatGPTUserID:         "user-123",
		ChatGPTAccountFedRAMP: true,
		PlanType:              "plus",
	})

	require.Equal(t, "at-test-token", credentials["access_token"])
	require.Equal(t, OpenAIAuthModePersonalAccessToken, credentials["auth_mode"])
	require.Equal(t, "personal_access_token", credentials["openai_auth_mode"])
	require.Equal(t, "Bearer", credentials["token_type"])
	require.Equal(t, true, credentials["chatgpt_account_is_fedramp"])
	require.NotContains(t, credentials, "expires_at")
	require.NotContains(t, credentials, "refresh_token")
	require.NotContains(t, credentials, "id_token")
}

func TestNormalizeOpenAIPersonalAccessTokenCredentialsRemovesOAuthFields(t *testing.T) {
	account := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"auth_mode": "personal_access_token",
		},
	}
	credentials := map[string]any{
		"access_token":                "at-test-token",
		"refresh_token":               "stale-refresh-token",
		"id_token":                    "stale-id-token",
		"expires_at":                  "2026-01-01T00:00:00Z",
		"expires_in":                  3600,
		"client_id":                   "stale-client",
		"model_mapping":               map[string]any{"gpt-5": "gpt-5-codex"},
		"chatgpt_account_is_fedramp":  true,
		"subscription_expires_at":     "2026-12-31T00:00:00Z",
		"openai_usage_channel_fields": []any{"custom"},
	}

	got := NormalizeOpenAIPersonalAccessTokenCredentials(account, nil, credentials)

	require.Equal(t, "at-test-token", got["access_token"])
	require.Equal(t, OpenAIAuthModePersonalAccessToken, got["auth_mode"])
	require.Equal(t, "personal_access_token", got["openai_auth_mode"])
	require.Equal(t, "Bearer", got["token_type"])
	require.NotContains(t, got, "refresh_token")
	require.NotContains(t, got, "id_token")
	require.NotContains(t, got, "expires_at")
	require.NotContains(t, got, "expires_in")
	require.NotContains(t, got, "client_id")
	require.Equal(t, map[string]any{"gpt-5": "gpt-5-codex"}, got["model_mapping"])
	require.Equal(t, true, got["chatgpt_account_is_fedramp"])
	require.Equal(t, "2026-12-31T00:00:00Z", got["subscription_expires_at"])
	require.Equal(t, []any{"custom"}, got["openai_usage_channel_fields"])
}
