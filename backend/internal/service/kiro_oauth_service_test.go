//go:build unit

package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
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

func TestKiroOAuthService_GenerateAuthURLCreatesExternalIdpSession(t *testing.T) {
	svc := NewKiroOAuthService(nil)

	result, err := svc.GenerateAuthURL(context.Background(), &KiroGenerateAuthURLInput{
		Provider: kiropkg.ProviderExternalIdp,
	})

	require.NoError(t, err)
	require.NotEmpty(t, result.AuthURL)
	require.NotEmpty(t, result.SessionID)
	require.NotEmpty(t, result.State)
	session, ok := svc.sessionStore.Get(result.SessionID)
	require.True(t, ok)
	require.Equal(t, "external_idp", session.AuthType)
	require.Equal(t, kiropkg.ProviderExternalIdp, session.Provider)
	require.Equal(t, kiroSocialRedirectURI, session.RedirectURI)
}

func TestKiroOAuthService_ExchangeCodeDoesNotSpecialCaseExternalIdpDescriptorInSocialSession(t *testing.T) {
	svc := NewKiroOAuthService(nil)
	svc.sessionStore.Set("social-session", &kiropkg.AuthSession{
		State:        "expected-state",
		CodeVerifier: "verifier",
		CreatedAt:    time.Now(),
		AuthType:     "social",
		Provider:     string(kiropkg.SocialProviderGoogle),
		RedirectURI:  kiroSocialRedirectURI,
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := svc.ExchangeCode(ctx, &KiroExchangeCodeInput{
		SessionID: "social-session",
		State:     "expected-state",
		Code:      "http://localhost:49153/signin/callback?login_option=external_idp&issuer_url=https%3A%2F%2Flogin.microsoftonline.com%2Ftenant%2Fv2.0&client_id=client-id&scopes=openid+profile",
	})

	require.ErrorIs(t, err, context.Canceled)
}

func TestKiroOAuthService_ExchangeCodePreparesExternalIdpAuthorizationFromDescriptor(t *testing.T) {
	previous := kiroDiscoverExternalIdp
	kiroDiscoverExternalIdp = func(ctx context.Context, proxyURL, issuerURL string) (string, string, error) {
		require.Equal(t, "", proxyURL)
		require.Equal(t, "https://login.microsoftonline.com/tenant/v2.0", issuerURL)
		return "https://login.microsoftonline.com/tenant/oauth2/v2.0/authorize", "https://login.microsoftonline.com/tenant/oauth2/v2.0/token", nil
	}
	t.Cleanup(func() { kiroDiscoverExternalIdp = previous })

	svc := NewKiroOAuthService(nil)
	svc.sessionStore.Set("external-session", &kiropkg.AuthSession{
		State:        "expected-state",
		CodeVerifier: "initial-verifier",
		CreatedAt:    time.Now(),
		AuthType:     "external_idp",
		Provider:     kiropkg.ProviderExternalIdp,
		RedirectURI:  kiroSocialRedirectURI,
	})

	issuerURL := "https://login.microsoftonline.com/tenant/v2.0"
	result, err := svc.ExchangeCode(context.Background(), &KiroExchangeCodeInput{
		SessionID: "external-session",
		State:     "expected-state",
		Code:      "http://localhost:49153/signin/callback?login_option=external_idp&issuer_url=" + url.QueryEscape(issuerURL) + "&client_id=client-id&scopes=openid+profile+offline_access&login_hint=user%40example.com",
	})

	require.NoError(t, err)
	require.Equal(t, "external-session", result.SessionID)
	require.NotEqual(t, "expected-state", result.State)
	require.Contains(t, result.AuthURL, "https://login.microsoftonline.com/tenant/oauth2/v2.0/authorize?")
	require.Contains(t, result.AuthURL, "client_id=client-id")
	require.Contains(t, result.AuthURL, "redirect_uri=http%3A%2F%2Flocalhost%3A3128%2Foauth%2Fcallback")
	require.Contains(t, result.AuthURL, "scope=openid+profile+offline_access")
	require.Contains(t, result.AuthURL, "login_hint=user%40example.com")

	session, ok := svc.sessionStore.Get("external-session")
	require.True(t, ok)
	require.Equal(t, "external_idp", session.AuthType)
	require.Equal(t, kiropkg.ProviderExternalIdp, session.Provider)
	require.Equal(t, "client-id", session.ClientID)
	require.Equal(t, "https://login.microsoftonline.com/tenant/oauth2/v2.0/token", session.TokenEndpoint)
	require.Equal(t, issuerURL, session.IssuerURL)
	require.Equal(t, "openid profile offline_access", session.Scopes)
	require.Equal(t, "user@example.com", session.LoginHint)
	require.Equal(t, kiroExternalIdpRedirectURI, session.RedirectURI)
	require.NotEqual(t, "initial-verifier", session.CodeVerifier)
	require.Equal(t, result.State, session.State)
}

func TestKiroOAuthService_ExchangeCodeRejectsFinalExternalIdpCodeBeforeDescriptor(t *testing.T) {
	svc := NewKiroOAuthService(nil)
	svc.sessionStore.Set("external-session", &kiropkg.AuthSession{
		State:        "expected-state",
		CodeVerifier: "verifier",
		CreatedAt:    time.Now(),
		AuthType:     "external_idp",
		Provider:     kiropkg.ProviderExternalIdp,
		RedirectURI:  kiroSocialRedirectURI,
	})

	_, err := svc.ExchangeCode(context.Background(), &KiroExchangeCodeInput{
		SessionID: "external-session",
		State:     "expected-state",
		Code:      "final-code",
	})

	require.EqualError(t, err, "kiro external_idp callback descriptor is required")
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

func TestParseKiroExternalIdpDescriptorFromCallbackURL(t *testing.T) {
	descriptor, ok := parseKiroExternalIdpDescriptor("http://localhost:49153/signin/callback?login_option=external_idp&issuer_url=https%3A%2F%2Flogin.example.com%2Ftenant%2Fv2.0&client_id=client-id&scopes=openid+profile+offline_access&login_hint=user%40example.com")

	require.True(t, ok)
	require.Equal(t, "client-id", descriptor.ClientID)
	require.Equal(t, "https://login.example.com/tenant/v2.0", descriptor.IssuerURL)
	require.Equal(t, "openid profile offline_access", descriptor.Scopes)
	require.Equal(t, "user@example.com", descriptor.LoginHint)
}

func TestParseKiroExternalIdpDescriptorIgnoresOAuthCallback(t *testing.T) {
	_, ok := parseKiroExternalIdpDescriptor("http://localhost:49153/oauth/callback?issuer_url=https%3A%2F%2Flogin.example.com%2Ftenant%2Fv2.0&client_id=client-id")

	require.False(t, ok)
}

func TestKiroOAuthService_RefreshTokenUsesExternalIdpTokenEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.NoError(t, r.ParseForm())
		require.Equal(t, "refresh_token", r.Form.Get("grant_type"))
		require.Equal(t, "refresh-token", r.Form.Get("refresh_token"))
		require.Equal(t, "client-id", r.Form.Get("client_id"))
		require.Equal(t, "openid profile offline_access", r.Form.Get("scope"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"new-access-token","expires_in":3600}`))
	}))
	defer server.Close()

	svc := NewKiroOAuthService(nil)
	token, err := svc.RefreshToken(context.Background(), &KiroRefreshTokenInput{
		AuthMethod:    "external_idp",
		RefreshToken:  "refresh-token",
		ClientID:      "client-id",
		TokenEndpoint: server.URL,
		IssuerURL:     "https://login.example.com/tenant/v2.0",
		Scopes:        "openid profile offline_access",
		ProfileArn:    "arn:aws:codewhisperer:us-east-1:123456789012:profile/EXTERNAL",
	})

	require.NoError(t, err)
	require.Equal(t, "new-access-token", token.AccessToken)
	require.Equal(t, "refresh-token", token.RefreshToken)
	require.Equal(t, "external_idp", token.AuthMethod)
	require.Equal(t, kiropkg.ProviderExternalIdp, token.Provider)
	require.Equal(t, "client-id", token.ClientID)
	require.Equal(t, server.URL, token.TokenEndpoint)
	require.Equal(t, "https://login.example.com/tenant/v2.0", token.IssuerURL)
	require.Equal(t, "openid profile offline_access", token.Scopes)
	require.Equal(t, "arn:aws:codewhisperer:us-east-1:123456789012:profile/EXTERNAL", token.ProfileArn)
}

func TestKiroOAuthService_BuildAccountCredentialsPreservesExternalIdpMetadata(t *testing.T) {
	svc := NewKiroOAuthService(nil)

	credentials := svc.BuildAccountCredentials(&KiroTokenInfo{
		AccessToken:   "access-token",
		RefreshToken:  "refresh-token",
		AuthMethod:    "external_idp",
		Provider:      kiropkg.ProviderExternalIdp,
		ClientID:      "client-id",
		TokenEndpoint: "https://login.example.com/oauth2/v2.0/token",
		IssuerURL:     "https://login.example.com/tenant/v2.0",
		Scopes:        "openid profile offline_access",
	})

	require.Equal(t, "external_idp", credentials["auth_method"])
	require.Equal(t, kiropkg.ProviderExternalIdp, credentials["provider"])
	require.Equal(t, "client-id", credentials["client_id"])
	require.Equal(t, "https://login.example.com/oauth2/v2.0/token", credentials["token_endpoint"])
	require.Equal(t, "https://login.example.com/tenant/v2.0", credentials["issuer_url"])
	require.Equal(t, "openid profile offline_access", credentials["scopes"])
}

func TestKiroOAuthService_RefreshTokenRejectsExternalIdpMissingMetadata(t *testing.T) {
	svc := NewKiroOAuthService(nil)

	_, err := svc.RefreshToken(context.Background(), &KiroRefreshTokenInput{
		AuthMethod:   "external_idp",
		RefreshToken: "refresh-token",
		ClientID:     "client-id",
	})

	require.EqualError(t, err, "kiro external_idp refresh requires client_id and token_endpoint")
}
