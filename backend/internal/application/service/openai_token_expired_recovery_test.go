package service

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExtractUpstreamErrorCode_TokenExpiredEnvelope(t *testing.T) {
	body := []byte(`{
  "detail": {"code": "token_expired", "message": "Your authentication token has expired."},
  "error": {"code": "token_expired", "message": "Your authentication token has expired.", "param": null, "type": "invalid_request_error"},
  "status": 401
}`)

	require.Equal(t, "token_expired", extractUpstreamErrorCode(body))
}

func TestIsOpenAIWSDialTokenExpired(t *testing.T) {
	err := &openAIWSDialError{
		StatusCode:   http.StatusUnauthorized,
		ResponseBody: []byte(`{"error":{"code":"token_expired"}}`),
	}

	require.True(t, isOpenAIWSDialTokenExpired(err))
	require.False(t, isOpenAIWSDialTokenExpired(&openAIWSDialError{
		StatusCode:   http.StatusUnauthorized,
		ResponseBody: []byte(`{"error":{"code":"token_revoked"}}`),
	}))
}

func TestIsOpenAIWSTokenExpiredEvent(t *testing.T) {
	require.True(t, isOpenAIWSTokenExpiredEvent([]byte(`{"type":"error","error":{"code":"token_expired"}}`)))
	require.False(t, isOpenAIWSTokenExpiredEvent([]byte(`{"type":"error","error":{"code":"token_revoked"}}`)))
}

func TestShouldRefreshOpenAIOAuth401_TeamGenericUnauthorizedIsRecoverable(t *testing.T) {
	account := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token":  "old-access-token",
			"refresh_token": "refresh-token",
		},
	}

	require.True(t, shouldRefreshOpenAIOAuth401(account, http.StatusUnauthorized, []byte(`{"detail":"Unauthorized"}`)))
	require.True(t, shouldRefreshOpenAIOAuth401(account, http.StatusUnauthorized, []byte(`{"error":{"code":"token_expired"}}`)))
	require.False(t, shouldRefreshOpenAIOAuth401(account, http.StatusUnauthorized, []byte(`{"error":{"code":"token_revoked"}}`)))
	require.False(t, shouldRefreshOpenAIOAuth401(account, http.StatusUnauthorized, []byte(`{"error":{"code":"invalid_api_key"}}`)))
	require.False(t, shouldRefreshOpenAIOAuth401(account, http.StatusBadRequest, []byte(`{"detail":"Unauthorized"}`)))

	account.Credentials["auth_mode"] = OpenAIAuthModePersonalAccessToken
	require.False(t, shouldRefreshOpenAIOAuth401(account, http.StatusUnauthorized, []byte(`{"detail":"Unauthorized"}`)))
}

func TestIsOpenAIWSTokenAuthRecoveryCandidate_OnlyAuthErrors(t *testing.T) {
	account := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"refresh_token": "refresh-token",
		},
	}

	require.True(t, isOpenAIWSTokenAuthRecoveryCandidate([]byte(`{"type":"error","error":{"code":"token_expired"}}`), account))
	require.True(t, isOpenAIWSTokenAuthRecoveryCandidate([]byte(`{"type":"error","error":{"message":"Unauthorized"}}`), account))
	require.False(t, isOpenAIWSTokenAuthRecoveryCandidate([]byte(`{"type":"error","error":{"code":"server_error","message":"try again"}}`), account))
}
