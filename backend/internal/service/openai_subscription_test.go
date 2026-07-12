package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/imroc/req/v3"
	"github.com/stretchr/testify/require"
)

func TestFetchChatGPTSubscriptionExpiresAt(t *testing.T) {
	const wantExpiresAt = "2026-06-10T02:52:15Z"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/backend-api/subscriptions", r.URL.Path)
		require.Equal(t, "acc_123", r.URL.Query().Get("account_id"))
		require.Equal(t, "Bearer access-token", r.Header.Get("Authorization"))

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"plan_type":    "plus",
			"active_until": wantExpiresAt,
			"will_renew":   true,
			"id":           "sub_123",
		})
	}))
	defer server.Close()

	oldURL := chatGPTSubscriptionsURL
	chatGPTSubscriptionsURL = server.URL + "/backend-api/subscriptions"
	t.Cleanup(func() { chatGPTSubscriptionsURL = oldURL })

	got := fetchChatGPTSubscriptionExpiresAt(context.Background(), func(proxyURL string) (*req.Client, error) {
		return req.C().SetTimeout(5 * time.Second), nil
	}, "access-token", "", "acc_123")

	require.Equal(t, wantExpiresAt, got)
}

func TestFetchChatGPTAccountInfo_SkipsExpiredWorkspaceCandidate(t *testing.T) {
	expiredAt := time.Now().Add(-24 * time.Hour).UTC().Format(time.RFC3339)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/backend-api/accounts/check/v4-2023-04-27", r.URL.Path)
		require.Equal(t, "Bearer access-token", r.Header.Get("Authorization"))

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"accounts": map[string]any{
				"org-expired-workspace": map[string]any{
					"account": map[string]any{
						"plan_type":  "self_serve_business_usage_based",
						"is_default": true,
					},
					"entitlement": map[string]any{
						"expires_at": expiredAt,
					},
				},
				"personal-account": map[string]any{
					"account": map[string]any{
						"plan_type": "free",
					},
				},
			},
		})
	}))
	defer server.Close()

	oldURL := chatGPTAccountsCheckURL
	chatGPTAccountsCheckURL = server.URL + "/backend-api/accounts/check/v4-2023-04-27"
	t.Cleanup(func() { chatGPTAccountsCheckURL = oldURL })

	got := fetchChatGPTAccountInfo(context.Background(), func(proxyURL string) (*req.Client, error) {
		return req.C().SetTimeout(5 * time.Second), nil
	}, "access-token", "", "org-expired-workspace")

	require.NotNil(t, got)
	require.Equal(t, "free", got.PlanType)
	require.Empty(t, got.SubscriptionExpiresAt)
}

func TestFetchChatGPTAccountInfo_SkipsDeactivatedWorkspaceCandidate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/backend-api/accounts/check/v4-2023-04-27", r.URL.Path)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"accounts": map[string]any{
				"org-deactivated-workspace": map[string]any{
					"account": map[string]any{
						"plan_type":      "self_serve_business_usage_based",
						"is_default":     true,
						"is_deactivated": true,
					},
				},
				"personal-account": map[string]any{
					"account": map[string]any{
						"plan_type": "pro",
					},
				},
			},
		})
	}))
	defer server.Close()

	oldURL := chatGPTAccountsCheckURL
	chatGPTAccountsCheckURL = server.URL + "/backend-api/accounts/check/v4-2023-04-27"
	t.Cleanup(func() { chatGPTAccountsCheckURL = oldURL })

	got := fetchChatGPTAccountInfo(context.Background(), func(proxyURL string) (*req.Client, error) {
		return req.C().SetTimeout(5 * time.Second), nil
	}, "access-token", "", "org-deactivated-workspace")

	require.NotNil(t, got)
	require.Equal(t, "pro", got.PlanType)
}

func TestShouldApplyChatGPTAccountInfoPlanType(t *testing.T) {
	require.False(t, shouldApplyChatGPTAccountInfoPlanType("pro", "self_serve_business_usage_based"))
	require.False(t, shouldApplyChatGPTAccountInfoPlanType("free", "team"))
	require.False(t, shouldApplyChatGPTAccountInfoPlanType("", ""))
	require.True(t, shouldApplyChatGPTAccountInfoPlanType("", "pro"))
}
