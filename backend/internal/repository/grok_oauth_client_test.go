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

func TestGrokOAuthClientDeviceAndRefreshUseCLIFormAndHeaders(t *testing.T) {
	var sawDevice, sawRefresh, sawExchange bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.NoError(t, r.ParseForm())
		require.Equal(t, "client-id", r.Form.Get("client_id"))
		require.Equal(t, xai.EffectiveCLIVersion(), r.Header.Get(xai.CLIClientVersionHeader))
		require.Equal(t, xai.CLIClientSurfaceValue, r.Header.Get(xai.CLIClientSurfaceHeader))
		require.Contains(t, r.Header.Get("User-Agent"), "grok-shell/")

		switch {
		case strings.HasSuffix(r.URL.Path, "/device/code") || r.Form.Get("grant_type") == "":
			if r.Form.Get("grant_type") == "" && r.Form.Get("scope") != "" {
				sawDevice = true
				require.Equal(t, xai.DefaultReferrer, r.Form.Get("referrer"))
				require.Contains(t, r.Form.Get("scope"), "conversations:write")
				_ = json.NewEncoder(w).Encode(map[string]any{
					"device_code":                "device-1",
					"user_code":                  "USER-1",
					"verification_uri":           "https://auth.x.ai/oauth2/device",
					"verification_uri_complete":  "https://auth.x.ai/oauth2/device?user_code=USER-1",
					"expires_in":                 600,
					"interval":                   5,
				})
				return
			}
			fallthrough
		default:
			switch r.Form.Get("grant_type") {
			case xai.DeviceGrantType:
				require.Equal(t, "device-1", r.Form.Get("device_code"))
				_ = json.NewEncoder(w).Encode(map[string]any{
					"access_token":  "device-access",
					"refresh_token": "device-refresh",
					"token_type":    "Bearer",
					"expires_in":    3600,
				})
			case "authorization_code":
				sawExchange = true
				require.Equal(t, "auth-code", r.Form.Get("code"))
				require.Equal(t, "http://127.0.0.1:56121/callback", r.Form.Get("redirect_uri"))
				require.Equal(t, "verifier", r.Form.Get("code_verifier"))
				_ = json.NewEncoder(w).Encode(map[string]any{
					"access_token":  "exchange-access",
					"refresh_token": "exchange-refresh",
					"token_type":    "Bearer",
					"expires_in":    3600,
					"scope":         "openid api:access",
				})
			case "refresh_token":
				sawRefresh = true
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
		}
	}))
	defer server.Close()
	t.Setenv(xai.EnvTokenURL, server.URL)
	t.Setenv(xai.EnvDeviceCodeURL, server.URL+"/device/code")

	client := NewGrokOAuthClient()

	device, err := client.RequestDeviceCode(context.Background(), "", "client-id", xai.DefaultScope)
	require.NoError(t, err)
	require.True(t, sawDevice)
	require.Equal(t, "device-1", device.DeviceCode)
	require.Equal(t, "USER-1", device.UserCode)

	poll, err := client.PollDeviceToken(context.Background(), "device-1", "", "client-id")
	require.NoError(t, err)
	require.Equal(t, xai.DevicePollAuthorized, poll.Status)
	require.Equal(t, "device-access", poll.Token.AccessToken)
	require.Equal(t, "device-refresh", poll.Token.RefreshToken)

	exchanged, err := client.ExchangeCode(
		context.Background(),
		"auth-code",
		"verifier",
		"http://127.0.0.1:56121/callback",
		"",
		"client-id",
	)
	require.NoError(t, err)
	require.True(t, sawExchange)
	require.Equal(t, "exchange-access", exchanged.AccessToken)
	require.Equal(t, "exchange-refresh", exchanged.RefreshToken)

	refreshed, err := client.RefreshToken(context.Background(), "refresh-token", "", "client-id")
	require.NoError(t, err)
	require.True(t, sawRefresh)
	require.Equal(t, "refresh-access", refreshed.AccessToken)
	require.Equal(t, "refresh-rotated", refreshed.RefreshToken)
	require.Equal(t, int64(7200), refreshed.ExpiresIn)
}

func TestGrokOAuthClientPollDevicePending(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"authorization_pending"}`))
	}))
	defer server.Close()
	t.Setenv(xai.EnvTokenURL, server.URL)

	client := NewGrokOAuthClient()
	poll, err := client.PollDeviceToken(context.Background(), "device-1", "", "client-id")
	require.NoError(t, err)
	require.Equal(t, xai.DevicePollPending, poll.Status)
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
