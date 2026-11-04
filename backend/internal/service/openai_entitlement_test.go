package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/imroc/req/v3"
	"github.com/stretchr/testify/require"
)

func TestChatGPTEntitlementExactAccount(t *testing.T) {
	expired := "2020-01-01T00:00:00Z"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer token", r.Header.Get("Authorization"))
		require.Equal(t, "personal", r.Header.Get("ChatGPT-Account-Id"))
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"accounts": map[string]any{
			"default": map[string]any{"account": map[string]any{"account_id": "personal", "plan_type": "pro"}, "entitlement": map[string]any{
				"expires_at": expired, "grace_period_end_timestamp": "2026-10-24T16:55:23+00:00", "grace_period_id": "pro_grace_period_28d_v2", "renews_at": "2026-11-10T00:00:00Z", "cancels_at": nil,
			}},
			"workspace": map[string]any{"account": map[string]any{"plan_type": "team"}, "entitlement": map[string]any{"expires_at": "2099-01-01T00:00:00Z"}},
		}})
	}))
	defer server.Close()
	old := chatGPTAccountsCheckURL
	chatGPTAccountsCheckURL = server.URL
	t.Cleanup(func() { chatGPTAccountsCheckURL = old })
	got := fetchChatGPTAccountInfo(context.Background(), func(string) (*req.Client, error) { return req.C(), nil }, "token", "", "workspace", "personal")
	require.NotNil(t, got)
	require.Equal(t, expired, got.Entitlement["expires_at"])
	require.Equal(t, "pro_grace_period_28d_v2", got.Entitlement["grace_period_id"])
	require.Nil(t, got.Entitlement["cancels_at"])
	require.NotEmpty(t, got.Entitlement["checked_at"])
	creds := (&OpenAIOAuthService{}).BuildAccountCredentials(&OpenAITokenInfo{ChatGPTEntitlement: got.Entitlement})
	require.Equal(t, got.Entitlement, creds["chatgpt_entitlement"])
}

func TestChatGPTEntitlementDoesNotUseOtherAccount(t *testing.T) {
	accounts := map[string]any{"default": map[string]any{"account": map[string]any{"account_id": "other"}, "entitlement": map[string]any{"expires_at": "2099-01-01T00:00:00Z"}}}
	require.Nil(t, extractRequestedChatGPTEntitlement(accounts, "personal"))
	accounts["personal"] = map[string]any{"entitlement": map[string]any{}}
	got := extractRequestedChatGPTEntitlement(accounts, "personal")
	require.Contains(t, got, "grace_period_end_timestamp")
	require.Nil(t, got["grace_period_end_timestamp"])
	require.Nil(t, got["expires_at"])
}

func TestRefreshAccountEntitlementStoredIdentity(t *testing.T) {
	for _, channel := range []string{"oauth", "web-image", "native"} {
		for _, scenario := range []string{"missing_id_token", "missing_claim", "fresh_identity", "cleared_fields", "other_account_only"} {
			t.Run(channel+"/"+scenario, func(t *testing.T) {
				requested := "personal"
				idToken := ""
				if scenario == "missing_claim" || scenario == "fresh_identity" {
					claims := map[string]any{"email": "test@example.com"}
					if scenario == "fresh_identity" {
						requested = "fresh-personal"
						claims["https://api.openai.com/auth"] = map[string]any{"chatgpt_account_id": requested}
					}
					payload, err := json.Marshal(claims)
					require.NoError(t, err)
					idToken = "e30." + base64.RawURLEncoding.EncodeToString(payload) + ".signature"
				}
				snapshot := map[string]any{
					"expires_at":                 "2098-11-10T00:00:00Z",
					"grace_period_end_timestamp": "2098-10-24T00:00:00Z",
					"renews_at":                  "2098-11-11T00:00:00Z", "cancels_at": nil,
				}
				if scenario == "cleared_fields" {
					snapshot = map[string]any{}
				}
				checks := 0
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					checks++
					require.Equal(t, "Bearer new-access", r.Header.Get("Authorization"))
					require.Equal(t, requested, r.Header.Get("ChatGPT-Account-Id"))
					accounts := map[string]any{
						"default": map[string]any{"account": map[string]any{"account_id": "workspace", "plan_type": "team"}, "entitlement": map[string]any{"expires_at": "2099-01-01T00:00:00Z"}},
					}
					if scenario != "other_account_only" {
						accounts[requested] = map[string]any{"account": map[string]any{"account_id": requested, "plan_type": "pro"}, "entitlement": snapshot}
					}
					w.Header().Set("Content-Type", "application/json")
					require.NoError(t, json.NewEncoder(w).Encode(map[string]any{"accounts": accounts}))
				}))
				defer server.Close()
				oldURL := chatGPTAccountsCheckURL
				chatGPTAccountsCheckURL = server.URL
				t.Cleanup(func() { chatGPTAccountsCheckURL = oldURL })
				client := &openaiOAuthClientRefreshStub{response: &openai.TokenResponse{AccessToken: "new-access", RefreshToken: "new-refresh", IDToken: idToken, ExpiresIn: 3600}}
				svc := NewOpenAIOAuthService(nil, client)
				defer svc.Stop()
				factoryCalls := 0
				svc.SetPrivacyClientFactory(func(string) (*req.Client, error) {
					factoryCalls++
					if factoryCalls > 1 {
						return nil, errors.New("block unrelated upstream calls in test")
					}
					return req.C(), nil
				})
				previous := map[string]any{"account_id": "personal", "expires_at": "2020-01-01T00:00:00Z", "cancels_at": "2020-01-01T00:00:00Z"}
				account := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{
					"access_token": "old-access", "refresh_token": "old-refresh", "chatgpt_account_id": "personal", "chatgpt_entitlement": previous,
				}}
				if channel == "web-image" {
					account.Type = AccountTypeWebImage
				}
				if channel == "native" {
					account.Extra = map[string]any{"openai_image_channel": "native"}
				}
				credentials, err := NewOpenAITokenRefresher(svc, nil).Refresh(context.Background(), account)
				require.NoError(t, err)
				require.Equal(t, int32(1), client.refreshCalls)
				require.Equal(t, 1, checks, "must enrich once, after filling the account ID")
				require.Equal(t, requested, credentials["chatgpt_account_id"])
				require.Equal(t, "new-access", credentials["access_token"])
				require.Equal(t, "new-refresh", credentials["refresh_token"])
				got, ok := credentials["chatgpt_entitlement"].(map[string]any)
				require.True(t, ok, "entitlement snapshot must be an object")
				if scenario == "other_account_only" {
					require.Equal(t, previous, got, "never replace the snapshot with another account's entitlement")
				} else {
					require.Equal(t, requested, got["account_id"])
					for _, key := range []string{"expires_at", "grace_period_end_timestamp", "renews_at", "cancels_at"} {
						require.Equal(t, snapshot[key], got[key], key)
					}
					require.NotEmpty(t, got["checked_at"])
				}
				require.Equal(t, previous, account.Credentials["chatgpt_entitlement"], "do not mutate the input account")
			})
		}
	}
}
