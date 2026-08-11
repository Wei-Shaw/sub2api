package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFeishuClientExchangeCodeForUserTokenParsesV3Response(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/oauth/v3/token", r.URL.Path)
		require.Equal(t, "application/json; charset=utf-8", r.Header.Get("Content-Type"))

		var body map[string]string
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		require.Equal(t, map[string]string{
			"grant_type":    "authorization_code",
			"client_id":     "test-app-id",
			"client_secret": "test-app-secret",
			"code":          "test-code",
			"redirect_uri":  "https://gateway.example.com/api/v1/auth/oauth/feishu/callback",
		}, body)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"msg":"success","access_token":"test-user-access","refresh_token":"test-user-refresh","expires_in":7200,"token_type":"Bearer"}`))
	}))
	defer server.Close()

	client := &FeishuClient{
		cfg: feishuClientConfig{
			AppID:     "test-app-id",
			AppSecret: "test-app-secret",
			TokenURL:  server.URL + "/oauth/v3/token",
		},
		httpClient: server.Client(),
	}

	token, err := client.ExchangeCodeForUserToken(context.Background(), "test-code", "https://gateway.example.com/api/v1/auth/oauth/feishu/callback")
	require.NoError(t, err)
	require.Equal(t, "test-user-access", token.AccessToken)
	require.Equal(t, "test-user-refresh", token.RefreshToken)
	require.Equal(t, int64(7200), token.ExpiresIn)
	require.Equal(t, "Bearer", token.TokenType)
}

func TestFeishuClientGetUserInfoPrefersEnterpriseEmail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "Bearer test-user-access", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"msg":"success","data":{"open_id":"open-test","union_id":"union-test","name":"Test User","email":"personal@example.com","enterprise_email":"member@company.example","avatar_url":"https://cdn.example.com/avatar.png","tenant_key":"tenant-test"}}`))
	}))
	defer server.Close()

	client := &FeishuClient{
		cfg:        feishuClientConfig{UserInfoURL: server.URL + "/open-apis/authen/v1/user_info"},
		httpClient: server.Client(),
	}

	user, err := client.GetUserInfo(context.Background(), "test-user-access")
	require.NoError(t, err)
	require.Equal(t, "union-test", user.UnionID)
	require.Equal(t, "tenant-test", user.TenantKey)
	require.Equal(t, "member@company.example", user.ResolvedEmail())
}
