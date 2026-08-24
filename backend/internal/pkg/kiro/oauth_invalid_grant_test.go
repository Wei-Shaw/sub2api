package kiro

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRefreshSocialTokenInvalidGrantReturnsTypedError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/refreshToken", r.URL.Path)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant","message":"Invalid refresh token provided"}`))
	}))
	defer server.Close()

	previous := socialAuthEndpointURL
	socialAuthEndpointURL = server.URL
	t.Cleanup(func() { socialAuthEndpointURL = previous })

	_, err := RefreshSocialToken(context.Background(), "", "revoked-refresh-token", "Google")
	require.Error(t, err)

	var invalid *RefreshTokenInvalidError
	require.True(t, errors.As(err, &invalid))
	require.Equal(t, http.StatusBadRequest, invalid.StatusCode)
	require.Contains(t, invalid.Body, "invalid_grant")
}

func TestRefreshIDCTokenInvalidGrantReturnsTypedError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/token", r.URL.Path)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant","message":"Invalid refresh token provided"}`))
	}))
	defer server.Close()

	previous := oidcEndpointOverride
	oidcEndpointOverride = server.URL
	t.Cleanup(func() { oidcEndpointOverride = previous })

	_, err := RefreshIDCToken(context.Background(), "", "client-id", "client-secret", "revoked-refresh-token", "us-east-1", BuilderIDStartURL, ProviderBuilderId)
	require.Error(t, err)

	var invalid *RefreshTokenInvalidError
	require.True(t, errors.As(err, &invalid))
	require.Equal(t, http.StatusBadRequest, invalid.StatusCode)
	require.Contains(t, invalid.Body, "invalid_grant")
}

func TestExchangeIDCAuthCodePreservesProfileArn(t *testing.T) {
	const profileArn = "arn:aws:codewhisperer:us-east-1:123456789012:profile/EXCHANGE"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"accessToken":"access-token","refreshToken":"refresh-token","profileArn":"` + profileArn + `","expiresIn":3600}`))
		case "/userinfo":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"email":"kiro@example.com"}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	previous := oidcEndpointOverride
	oidcEndpointOverride = server.URL
	t.Cleanup(func() { oidcEndpointOverride = previous })

	token, err := ExchangeIDCAuthCode(context.Background(), "", "client-id", "client-secret", "code", "verifier", "http://127.0.0.1:9876/oauth/callback", "us-east-1", BuilderIDStartURL)
	require.NoError(t, err)
	require.Equal(t, profileArn, token.ProfileArn)
	require.Equal(t, "kiro@example.com", token.Email)
}

func TestRefreshIDCTokenPreservesProfileArn(t *testing.T) {
	const profileArn = "arn:aws:codewhisperer:us-east-1:123456789012:profile/REFRESH"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"accessToken":"access-token","refreshToken":"refresh-token","profileArn":"` + profileArn + `","expiresIn":3600}`))
		case "/userinfo":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"email":"kiro@example.com"}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	previous := oidcEndpointOverride
	oidcEndpointOverride = server.URL
	t.Cleanup(func() { oidcEndpointOverride = previous })

	token, err := RefreshIDCToken(context.Background(), "", "client-id", "client-secret", "refresh-token", "us-east-1", BuilderIDStartURL, ProviderBuilderId)
	require.NoError(t, err)
	require.Equal(t, profileArn, token.ProfileArn)
	require.Equal(t, "kiro@example.com", token.Email)
}

// TestRefreshIDCTokenPreservesEnterpriseProviderWithoutStartURL 防退化回归:
// 导入的 Enterprise 账号无 startURL,刷新时必须保留存量 Enterprise,不得退化为 BuilderId。
func TestRefreshIDCTokenPreservesEnterpriseProviderWithoutStartURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"accessToken":"access-token","refreshToken":"refresh-token","profileArn":"arn:x","expiresIn":3600}`))
		case "/userinfo":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"email":"kiro@example.com"}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	previous := oidcEndpointOverride
	oidcEndpointOverride = server.URL
	t.Cleanup(func() { oidcEndpointOverride = previous })

	// startURL 为空,但存量 provider 为 Enterprise → 必须保留 Enterprise。
	token, err := RefreshIDCToken(context.Background(), "", "client-id", "client-secret", "refresh-token", "us-east-1", "", ProviderEnterprise)
	require.NoError(t, err)
	require.Equal(t, ProviderEnterprise, token.Provider)

	// startURL 与 provider 都为空 → 兜底 BuilderId。
	token2, err := RefreshIDCToken(context.Background(), "", "client-id", "client-secret", "refresh-token", "us-east-1", "", "")
	require.NoError(t, err)
	require.Equal(t, ProviderBuilderId, token2.Provider)
}
