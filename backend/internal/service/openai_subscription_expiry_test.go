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

func newSubscriptionExpiryTestAccount(id int64, chatGPTAccountID string) *Account {
	return &Account{
		ID:       id,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token":       "test-access-token",
			"chatgpt_account_id": chatGPTAccountID,
			"expires_at":         time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
		},
	}
}

func newSubscriptionExpiryTestService(repo *stubQuotaAccountRepo, server *httptest.Server) *OpenAIQuotaService {
	return NewOpenAIQuotaService(
		repo,
		nil,
		&OpenAITokenProvider{},
		newQuotaRedirectingFactory(server),
	)
}

func TestQuerySubscriptionExpiry_SubscriptionsSuccess(t *testing.T) {
	const accountID = int64(41)
	const chatGPTAccountID = "chatgpt-account-41"
	const wantExpiresAt = "2026-08-08T07:23:45Z"

	var subscriptionCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/backend-api/subscriptions", r.URL.Path)
		require.Equal(t, chatGPTAccountID, r.URL.Query().Get("account_id"))
		require.Equal(t, "Bearer test-access-token", r.Header.Get("Authorization"))
		subscriptionCalls++
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"plan_type":    "plus",
			"active_until": wantExpiresAt,
			"will_renew":   false,
		})
	}))
	defer server.Close()

	account := newSubscriptionExpiryTestAccount(accountID, chatGPTAccountID)
	repo := &stubQuotaAccountRepo{accounts: map[int64]*Account{accountID: account}}
	result, err := newSubscriptionExpiryTestService(repo, server).QuerySubscriptionExpiry(context.Background(), accountID)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, accountID, result.AccountID)
	require.Equal(t, OpenAISubscriptionExpiryStatusAvailable, result.Snapshot.Status)
	require.Equal(t, wantExpiresAt, result.Snapshot.ExpiresAt)
	require.Equal(t, OpenAISubscriptionExpirySourceSubscriptions, result.Snapshot.Source)
	require.Equal(t, "plus", result.Snapshot.PlanType)
	require.False(t, result.Snapshot.WillRenew)
	require.Equal(t, wantExpiresAt, result.EffectiveExpiresAt)
	require.Equal(t, OpenAISubscriptionExpiryEffectiveSourceUpstream, result.EffectiveSource)
	require.Len(t, repo.extraUpdates, 1)
	require.Equal(t, result.Snapshot, repo.extraUpdates[accountID][OpenAISubscriptionExpirySnapshotKey])
	require.Equal(t, 1, subscriptionCalls)
}

func TestQuerySubscriptionExpiry_SubscriptionsWithoutActiveUntilFallsBackToAccountsCheck(t *testing.T) {
	const accountID = int64(42)
	const chatGPTAccountID = "chatgpt-account-42"
	const wantExpiresAt = "2026-09-01T10:20:30+00:00"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/backend-api/subscriptions":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"plan_type":    "plus",
				"active_until": "",
				"will_renew":   true,
			})
		case "/backend-api/accounts/check/v4-2023-04-27":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"accounts": map[string]any{
					chatGPTAccountID: map[string]any{
						"account":     map[string]any{"plan_type": "plus"},
						"entitlement": map[string]any{"expires_at": wantExpiresAt},
					},
				},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	account := newSubscriptionExpiryTestAccount(accountID, chatGPTAccountID)
	repo := &stubQuotaAccountRepo{accounts: map[int64]*Account{accountID: account}}
	result, err := newSubscriptionExpiryTestService(repo, server).QuerySubscriptionExpiry(context.Background(), accountID)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, OpenAISubscriptionExpiryStatusAvailable, result.Snapshot.Status)
	require.Equal(t, "2026-09-01T10:20:30Z", result.Snapshot.ExpiresAt)
	require.Equal(t, OpenAISubscriptionExpirySourceAccountsCheck, result.Snapshot.Source)
	require.Equal(t, "plus", result.Snapshot.PlanType)
	require.True(t, result.Snapshot.WillRenew)
	require.Equal(t, OpenAISubscriptionExpiryEffectiveSourceUpstream, result.EffectiveSource)
}

func TestQuerySubscriptionExpiry_NoRealValueCachesUnavailableAndUsesManualEffectiveExpiry(t *testing.T) {
	const accountID = int64(43)
	const chatGPTAccountID = "chatgpt-account-43"
	manualExpiry := time.Date(2027, time.March, 4, 5, 6, 7, 0, time.FixedZone("CST", 8*60*60))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/backend-api/subscriptions":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"plan_type":    "free",
				"active_until": "",
				"will_renew":   false,
			})
		case "/backend-api/accounts/check/v4-2023-04-27":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"accounts": map[string]any{
					chatGPTAccountID: map[string]any{
						"account": map[string]any{"plan_type": "free"},
					},
				},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	account := newSubscriptionExpiryTestAccount(accountID, chatGPTAccountID)
	account.ExpiresAt = &manualExpiry
	repo := &stubQuotaAccountRepo{accounts: map[int64]*Account{accountID: account}}
	result, err := newSubscriptionExpiryTestService(repo, server).QuerySubscriptionExpiry(context.Background(), accountID)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, OpenAISubscriptionExpiryStatusUnavailable, result.Snapshot.Status)
	require.Empty(t, result.Snapshot.ExpiresAt)
	require.Equal(t, OpenAISubscriptionExpirySourceUnavailable, result.Snapshot.Source)
	require.False(t, result.Snapshot.WillRenew)
	require.Equal(t, manualExpiry.UTC().Format(time.RFC3339Nano), result.EffectiveExpiresAt)
	require.Equal(t, OpenAISubscriptionExpiryEffectiveSourceManual, result.EffectiveSource)
	require.Equal(t, result.Snapshot, repo.extraUpdates[accountID][OpenAISubscriptionExpirySnapshotKey])
}

func TestQuerySubscriptionExpiry_SubscriptionsEmptyActiveUntilIsTrustedWhenAccountsCheckFails(t *testing.T) {
	const accountID = int64(46)
	const chatGPTAccountID = "chatgpt-account-46"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/backend-api/subscriptions":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"plan_type":    "free",
				"active_until": "",
				"will_renew":   false,
			})
		case "/backend-api/accounts/check/v4-2023-04-27":
			w.WriteHeader(http.StatusBadGateway)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	account := newSubscriptionExpiryTestAccount(accountID, chatGPTAccountID)
	repo := &stubQuotaAccountRepo{accounts: map[int64]*Account{accountID: account}}
	result, err := newSubscriptionExpiryTestService(repo, server).QuerySubscriptionExpiry(context.Background(), accountID)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, OpenAISubscriptionExpiryStatusUnavailable, result.Snapshot.Status)
	require.Empty(t, result.Snapshot.ExpiresAt)
	require.Equal(t, OpenAISubscriptionExpirySourceUnavailable, result.Snapshot.Source)
	require.Equal(t, "free", result.Snapshot.PlanType)
	require.False(t, result.Snapshot.WillRenew)
	require.Equal(t, result.Snapshot, repo.extraUpdates[accountID][OpenAISubscriptionExpirySnapshotKey])
}

func TestQuerySubscriptionExpiry_BothUpstreamEndpointsFailPreservesOldCache(t *testing.T) {
	const accountID = int64(44)
	const chatGPTAccountID = "chatgpt-account-44"
	oldSnapshot := OpenAISubscriptionExpirySnapshot{
		Status:    OpenAISubscriptionExpiryStatusAvailable,
		ExpiresAt: "2026-01-01T00:00:00Z",
		CheckedAt: "2025-12-01T00:00:00Z",
		Source:    OpenAISubscriptionExpirySourceSubscriptions,
		PlanType:  "plus",
		WillRenew: true,
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()

	account := newSubscriptionExpiryTestAccount(accountID, chatGPTAccountID)
	repo := &stubQuotaAccountRepo{
		accounts:     map[int64]*Account{accountID: account},
		extraUpdates: map[int64]map[string]any{accountID: {OpenAISubscriptionExpirySnapshotKey: oldSnapshot}},
	}
	result, err := newSubscriptionExpiryTestService(repo, server).QuerySubscriptionExpiry(context.Background(), accountID)

	require.Nil(t, result)
	require.Error(t, err)
	require.Equal(t, oldSnapshot, repo.extraUpdates[accountID][OpenAISubscriptionExpirySnapshotKey])
}

func TestQuerySubscriptionExpiry_UnmatchedMultiAccountFallbackPreservesOldCache(t *testing.T) {
	const accountID = int64(47)
	const chatGPTAccountID = "target-chatgpt-account"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/backend-api/subscriptions":
			w.WriteHeader(http.StatusBadGateway)
		case "/backend-api/accounts/check/v4-2023-04-27":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"accounts": map[string]any{
					"other-one": map[string]any{"entitlement": map[string]any{"expires_at": "2027-01-01T00:00:00Z"}},
					"other-two": map[string]any{"entitlement": map[string]any{"expires_at": "2027-02-01T00:00:00Z"}},
				},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	account := newSubscriptionExpiryTestAccount(accountID, chatGPTAccountID)
	repo := &stubQuotaAccountRepo{accounts: map[int64]*Account{accountID: account}}
	result, err := newSubscriptionExpiryTestService(repo, server).QuerySubscriptionExpiry(context.Background(), accountID)

	require.Nil(t, result)
	require.Error(t, err)
	require.Empty(t, repo.extraUpdates)
}

func TestQuerySubscriptionExpiry_ShadowUsesParentCredentialsAndProxyButCachesShadowRow(t *testing.T) {
	const shadowID = int64(45)
	const chatGPTAccountID = "parent-chatgpt-account"
	parentID := int64(145)

	parent := newSubscriptionExpiryTestAccount(parentID, chatGPTAccountID)
	proxyID := int64(9)
	parent.ProxyID = &proxyID
	parent.Proxy = &Proxy{Protocol: "http", Host: "proxy.example", Port: 8080}
	parentExpiry := time.Date(2027, time.June, 7, 8, 9, 10, 0, time.UTC)
	parent.ExpiresAt = &parentExpiry
	shadow := &Account{
		ID:              shadowID,
		Platform:        PlatformOpenAI,
		Type:            AccountTypeOAuth,
		ParentAccountID: &parentID,
		QuotaDimension:  QuotaDimensionSpark,
	}
	repo := &stubQuotaAccountRepo{accounts: map[int64]*Account{shadowID: shadow, parentID: parent}}

	var seenProxyURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/backend-api/subscriptions", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"plan_type":    "plus",
			"active_until": "2027-07-08T09:10:11Z",
			"will_renew":   true,
		})
	}))
	defer server.Close()

	factory := func(proxyURL string) (*req.Client, error) {
		seenProxyURL = proxyURL
		return newQuotaRedirectingFactory(server)(proxyURL)
	}
	service := NewOpenAIQuotaService(repo, nil, &OpenAITokenProvider{}, factory)
	result, err := service.QuerySubscriptionExpiry(context.Background(), shadowID)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "http://proxy.example:8080", seenProxyURL)
	require.Contains(t, repo.extraUpdates, shadowID)
	require.NotContains(t, repo.extraUpdates, parentID)
	require.Equal(t, result.Snapshot, repo.extraUpdates[shadowID][OpenAISubscriptionExpirySnapshotKey])
}
