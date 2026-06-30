//go:build unit

package repository

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"github.com/stretchr/testify/require"
)

func TestGrokOAuthClientExchangeAndRefreshUseFormFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.NoError(t, r.ParseForm())
		require.Equal(t, "client-id", r.Form.Get("client_id"))

		switch r.Form.Get("grant_type") {
		case "authorization_code":
			require.Equal(t, "auth-code", r.Form.Get("code"))
			require.Equal(t, "http://127.0.0.1:56121/callback", r.Form.Get("redirect_uri"))
			require.Equal(t, "verifier", r.Form.Get("code_verifier"))
			require.Empty(t, r.Form.Get("code_challenge"))
			require.Empty(t, r.Form.Get("code_challenge_method"))
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token":  "exchange-access",
				"refresh_token": "exchange-refresh",
				"token_type":    "Bearer",
				"expires_in":    3600,
				"scope":         "openid api:access",
			})
		case "refresh_token":
			require.Equal(t, "refresh-token", r.Form.Get("refresh_token"))
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token":  "refresh-access",
				"refresh_token": "refresh-rotated",
				"token_type":    "Bearer",
				"expires_in":    7200,
			})
		default:
			http.Error(w, "unexpected grant_type", http.StatusBadRequest)
		}
	}))
	defer server.Close()
	t.Setenv(xai.EnvTokenURL, server.URL)

	client := NewGrokOAuthClient()

	exchanged, err := client.ExchangeCode(
		context.Background(),
		"auth-code",
		"verifier",
		"http://127.0.0.1:56121/callback",
		"",
		"client-id",
	)
	require.NoError(t, err)
	require.Equal(t, "exchange-access", exchanged.AccessToken)
	require.Equal(t, "exchange-refresh", exchanged.RefreshToken)
	require.Equal(t, int64(3600), exchanged.ExpiresIn)
	require.Equal(t, "openid api:access", exchanged.Scope)

	refreshed, err := client.RefreshToken(context.Background(), "refresh-token", "", "client-id")
	require.NoError(t, err)
	require.Equal(t, "refresh-access", refreshed.AccessToken)
	require.Equal(t, "refresh-rotated", refreshed.RefreshToken)
	require.Equal(t, int64(7200), refreshed.ExpiresIn)
}

func TestGrokOAuthClientRefreshForbiddenClassifiesEntitlement(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"subscription required"}`))
	}))
	defer server.Close()
	t.Setenv(xai.EnvTokenURL, server.URL)

	client := NewGrokOAuthClient()
	_, err := client.RefreshToken(context.Background(), "refresh-token", "", "client-id")
	require.Error(t, err)
	require.Contains(t, strings.ToUpper(err.Error()), "GROK_OAUTH_ENTITLEMENT_DENIED")
}

func TestGrokOAuthClientStatusErrorRedactsSensitiveResponseBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant","access_token":"access-secret","refresh_token":"refresh-secret","code_verifier":"verifier-secret"}`))
	}))
	defer server.Close()
	t.Setenv(xai.EnvTokenURL, server.URL)

	client := NewGrokOAuthClient()
	_, err := client.RefreshToken(context.Background(), "refresh-secret", "", "client-id")
	require.Error(t, err)

	errText := err.Error()
	require.Contains(t, errText, "status 400")
	require.Contains(t, errText, `\"refresh_token\":\"***\"`)
	require.NotContains(t, errText, "access-secret")
	require.NotContains(t, errText, "refresh-secret")
	require.NotContains(t, errText, "verifier-secret")
}
