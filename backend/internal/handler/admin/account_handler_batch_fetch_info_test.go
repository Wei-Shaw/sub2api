package admin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/imroc/req/v3"
	"github.com/stretchr/testify/require"
)

func newBatchFetchInfoTestClient(t *testing.T, serverURL string) *req.Client {
	t.Helper()

	target, err := url.Parse(serverURL)
	require.NoError(t, err)

	client := req.C()
	client.GetTransport().WrapRoundTripFunc(func(rt http.RoundTripper) req.HttpRoundTripFunc {
		return func(r *http.Request) (*http.Response, error) {
			cloned := r.Clone(r.Context())
			cloned.URL.Scheme = target.Scheme
			cloned.URL.Host = target.Host
			cloned.Host = target.Host
			return rt.RoundTrip(cloned)
		}
	})
	return client
}

func setupBatchFetchAccountInfoRouter(adminSvc *stubAdminService, openaiSvc *service.OpenAIOAuthService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	accountHandler := NewAccountHandler(adminSvc, nil, openaiSvc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	router.POST("/api/v1/admin/accounts/batch-fetch-account-info", accountHandler.BatchFetchAccountInfo)
	return router
}

func TestAccountHandlerBatchFetchAccountInfo(t *testing.T) {
	adminSvc := newStubAdminService()
	adminSvc.accounts = []service.Account{
		{
			ID:       1,
			Name:     "openai-oauth",
			Platform: service.PlatformOpenAI,
			Type:     service.AccountTypeOAuth,
			Status:   service.StatusActive,
			Credentials: map[string]any{
				"access_token":    "token-1",
				"organization_id": "org-1",
				"expires_at":      time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
				"model_mapping": map[string]any{
					"gpt-5.5": "gpt-5.4",
				},
				"gpt55_disabled_reason": "upstream_not_supported",
			},
		},
		{
			ID:       2,
			Name:     "anthropic-oauth",
			Platform: service.PlatformAnthropic,
			Type:     service.AccountTypeOAuth,
			Status:   service.StatusActive,
			Credentials: map[string]any{
				"access_token": "token-2",
			},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/backend-api/accounts/check/v4-2023-04-27":
			_, _ = w.Write([]byte(`{
				"accounts": {
					"org-1": {
						"account": {
							"plan_type": "pro",
							"is_default": true
						},
						"entitlement": {
							"expires_at": "2026-06-01T00:00:00Z"
						}
					}
				}
			}`))
		case "/backend-api/codex/models":
			_, _ = w.Write([]byte(`{
				"models": [
					{"slug": "gpt-5.5"},
					{"slug": "gpt-5.4-mini"}
				]
			}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	openaiSvc := service.NewOpenAIOAuthService(nil, nil)
	openaiSvc.SetPrivacyClientFactory(func(proxyURL string) (*req.Client, error) {
		return newBatchFetchInfoTestClient(t, server.URL), nil
	})

	router := setupBatchFetchAccountInfoRouter(adminSvc, openaiSvc)

	body, _ := json.Marshal(map[string]any{
		"account_ids": []int64{1, 2, 999},
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/batch-fetch-account-info", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, float64(0), resp["code"])

	data, ok := resp["data"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, float64(3), data["total"])
	require.Equal(t, float64(1), data["success"])
	require.Equal(t, float64(2), data["failed"])

	updated := adminSvc.updatedAccounts[1]
	require.NotNil(t, updated)
	require.Equal(t, "pro", updated.Credentials["plan_type"])
	require.Equal(t, "2026-06-01T00:00:00Z", updated.Credentials["subscription_expires_at"])
	require.Equal(t, []string{"gpt-5.4-mini", "gpt-5.5"}, updated.Credentials["supported_models"])
	require.NotEmpty(t, updated.Credentials["supported_models_synced_at"])
	require.Equal(t, map[string]any{"gpt-5.5": "gpt-5.4"}, updated.Credentials["model_mapping"])
	require.Equal(t, "upstream_not_supported", updated.Credentials["gpt55_disabled_reason"])
}
