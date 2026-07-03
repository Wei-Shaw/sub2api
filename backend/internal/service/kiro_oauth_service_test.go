//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	kiropkg "github.com/Wei-Shaw/sub2api/internal/pkg/kiro"
	"github.com/stretchr/testify/require"
)

func TestKiroIDCAuthRedirectURIUsesLoopbackIP(t *testing.T) {
	require.Equal(t, "http://127.0.0.1:9876/oauth/callback", kiroIDCRedirectURI)
}

func TestKiroSocialAuthRedirectURIUsesLoopbackIP(t *testing.T) {
	require.Equal(t, "http://localhost:49153", kiroSocialRedirectURI)
}

func TestBuildKiroSocialExchangeRedirectURIUsesProviderDefault(t *testing.T) {
	require.Equal(
		t,
		"http://localhost:49153/oauth/callback?login_option=github",
		buildKiroSocialExchangeRedirectURI("http://localhost:49153", "Github", "", ""),
	)
}

func TestBuildKiroSocialExchangeRedirectURIPreservesParsedCallbackData(t *testing.T) {
	require.Equal(
		t,
		"http://localhost:49153/signin/callback?login_option=google",
		buildKiroSocialExchangeRedirectURI("http://localhost:49153", "Github", "/signin/callback", "google"),
	)
}

func TestKiroOAuthService_ExchangeCodeRejectsExpiredSession(t *testing.T) {
	svc := NewKiroOAuthService(nil)
	svc.sessionStore.Set("expired-session", &kiropkg.AuthSession{
		State:     "expected-state",
		CreatedAt: time.Now().Add(-11 * time.Minute),
	})

	_, err := svc.ExchangeCode(context.Background(), &KiroExchangeCodeInput{
		SessionID: "expired-session",
		State:     "expected-state",
		Code:      "auth-code",
	})
	require.EqualError(t, err, "session not found or expired")
}

func TestKiroOAuthService_RefreshTokenRejectsMissingRefreshToken(t *testing.T) {
	svc := NewKiroOAuthService(nil)

	_, err := svc.RefreshToken(context.Background(), &KiroRefreshTokenInput{
		AuthMethod: "social",
	})

	require.EqualError(t, err, "kiro refresh token is required")
}

func TestKiroOAuthService_RefreshTokenRejectsIDCMissingClientCredentials(t *testing.T) {
	svc := NewKiroOAuthService(nil)

	_, err := svc.RefreshToken(context.Background(), &KiroRefreshTokenInput{
		AuthMethod:   "idc",
		RefreshToken: "refresh-token",
		ClientID:     "client-id",
	})

	require.EqualError(t, err, "kiro idc refresh requires client_id and client_secret")
}

func TestResolveKiroRefreshAuthMethodInfersIDCFromClientCredentials(t *testing.T) {
	require.Equal(t, "idc", resolveKiroRefreshAuthMethod("", "client-id", "client-secret"))
	require.Equal(t, "social", resolveKiroRefreshAuthMethod("", "client-id", ""))
	require.Equal(t, "social", resolveKiroRefreshAuthMethod("", "", "client-secret"))
	require.Equal(t, "social", resolveKiroRefreshAuthMethod("", "", ""))
	require.Equal(t, "idc", resolveKiroRefreshAuthMethod("IDC", "", ""))
}
