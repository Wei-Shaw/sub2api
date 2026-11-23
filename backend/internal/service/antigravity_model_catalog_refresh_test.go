//go:build unit

package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
)

// serveAntigravityCatalog points antigravity.BaseURLs at a server that answers
// fetchAvailableModels with the Ultra fixture plus uncallable entries.
func serveAntigravityCatalog(t *testing.T, status int) *atomic.Int32 {
	t.Helper()
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1internal:fetchAvailableModels" {
			http.NotFound(w, r)
			return
		}
		hits.Add(1)
		if status != http.StatusOK {
			w.WriteHeader(status)
			return
		}
		models := make(map[string]any, len(antigravityUltraCatalogFixture)+2)
		for _, id := range antigravityUltraCatalogFixture {
			models[id] = map[string]any{"quotaInfo": map[string]any{"remainingFraction": 1}}
		}
		models["chat_20706-disabled"] = map[string]any{}
		models["internal-preview"] = map[string]any{"isInternal": true}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"models": models})
	}))
	t.Cleanup(server.Close)

	oldBaseURLs := append([]string(nil), antigravity.BaseURLs...)
	oldAvailability := antigravity.DefaultURLAvailability
	t.Cleanup(func() {
		antigravity.BaseURLs = oldBaseURLs
		antigravity.DefaultURLAvailability = oldAvailability
	})
	antigravity.BaseURLs = []string{server.URL}
	antigravity.DefaultURLAvailability = antigravity.NewURLAvailability(time.Minute)
	return &hits
}

func TestTokenRefreshService_EnsureAntigravityModelCatalog(t *testing.T) {
	wantModels := normalizeAntigravityCatalogIDs(antigravityUltraCatalogFixture)
	cases := []struct {
		name      string
		extra     map[string]any
		wantFetch bool
	}{
		{name: "missing catalog is fetched", extra: nil, wantFetch: true},
		{
			name:      "stale catalog is refreshed",
			extra:     map[string]any{AntigravityModelCatalogExtraKey: decodedCatalog([]string{"gemini-3-flash"}, time.Now().Add(-7*time.Hour).UTC().Format(time.RFC3339))},
			wantFetch: true,
		},
		{
			name:      "unreadable synced_at is refreshed",
			extra:     map[string]any{AntigravityModelCatalogExtraKey: decodedCatalog([]string{"gemini-3-flash"}, "yesterday")},
			wantFetch: true,
		},
		{
			name:      "fresh catalog is kept",
			extra:     map[string]any{AntigravityModelCatalogExtraKey: decodedCatalog([]string{"gemini-3-flash"}, time.Now().Add(-time.Hour).UTC().Format(time.RFC3339))},
			wantFetch: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hits := serveAntigravityCatalog(t, http.StatusOK)
			repo := &tokenRefreshAccountRepo{}
			svc := NewTokenRefreshService(repo, nil, nil, nil, nil, nil, nil, &config.Config{}, nil)
			account := &Account{
				ID:       41,
				Platform: PlatformAntigravity,
				Type:     AccountTypeOAuth,
				Credentials: map[string]any{
					"access_token":                           "ag-token",
					"project_id":                             "proj",
					AntigravityModelMappingModeCredentialKey: AntigravityModelMappingModeLive,
				},
				Extra: tc.extra,
			}

			svc.ensureAntigravityModelCatalog(context.Background(), account)

			if !tc.wantFetch {
				require.Zero(t, hits.Load())
				require.Zero(t, repo.updateExtraCalls)
				return
			}
			require.Equal(t, int32(1), hits.Load())
			require.Equal(t, 1, repo.updateExtraCalls)
			saved, ok := repo.lastExtraUpdates[AntigravityModelCatalogExtraKey].(AntigravityModelCatalog)
			require.True(t, ok)
			require.Equal(t, wantModels, saved.Models)
			_, err := time.Parse(time.RFC3339, saved.SyncedAt)
			require.NoError(t, err)
			require.Equal(t, wantModels, account.antigravityModelCatalogIDs())
		})
	}
}

func TestTokenRefreshService_EnsureAntigravityModelCatalogKeepsCatalogOnFailure(t *testing.T) {
	hits := serveAntigravityCatalog(t, http.StatusInternalServerError)
	repo := &tokenRefreshAccountRepo{}
	svc := NewTokenRefreshService(repo, nil, nil, nil, nil, nil, nil, &config.Config{}, nil)
	previous := decodedCatalog([]string{"gemini-3-flash"}, time.Now().Add(-8*time.Hour).UTC().Format(time.RFC3339))
	account := &Account{
		ID:          42,
		Platform:    PlatformAntigravity,
		Type:        AccountTypeOAuth,
		Credentials: map[string]any{"access_token": "ag-token", AntigravityModelMappingModeCredentialKey: AntigravityModelMappingModeLive},
		Extra:       map[string]any{AntigravityModelCatalogExtraKey: previous},
	}

	svc.ensureAntigravityModelCatalog(context.Background(), account)

	require.Equal(t, int32(1), hits.Load())
	require.Zero(t, repo.updateExtraCalls)
	require.Equal(t, previous, account.Extra[AntigravityModelCatalogExtraKey])
}

func TestTokenRefreshService_EnsureAntigravityModelCatalogSkipsOtherAccounts(t *testing.T) {
	hits := serveAntigravityCatalog(t, http.StatusOK)
	repo := &tokenRefreshAccountRepo{}
	svc := NewTokenRefreshService(repo, nil, nil, nil, nil, nil, nil, &config.Config{}, nil)
	live := AntigravityModelMappingModeLive
	for _, account := range []*Account{
		// Accounts outside live mode get their catalog from the quota fetch and the sync button only.
		{ID: 1, Platform: PlatformAntigravity, Type: AccountTypeOAuth, Credentials: map[string]any{"access_token": "t"}},
		{ID: 2, Platform: PlatformAntigravity, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "k", AntigravityModelMappingModeCredentialKey: live}},
		{ID: 3, Platform: PlatformGemini, Type: AccountTypeOAuth, Credentials: map[string]any{"access_token": "t", AntigravityModelMappingModeCredentialKey: live}},
		{ID: 4, Platform: PlatformAntigravity, Type: AccountTypeOAuth, Credentials: map[string]any{AntigravityModelMappingModeCredentialKey: live}},
	} {
		svc.ensureAntigravityModelCatalog(context.Background(), account)
	}
	require.Zero(t, hits.Load())
	require.Zero(t, repo.updateExtraCalls)
}

func TestTokenRefreshService_EnsureAntigravityModelCatalogNeverBypassesBoundProxy(t *testing.T) {
	hits := serveAntigravityCatalog(t, http.StatusOK)
	proxyID := int64(9)
	for name, lookup := range map[string]func(context.Context, int64) (*Proxy, error){
		"lookup error":  func(context.Context, int64) (*Proxy, error) { return nil, errors.New("db down") },
		"missing proxy": func(context.Context, int64) (*Proxy, error) { return nil, nil },
	} {
		t.Run(name, func(t *testing.T) {
			repo := &tokenRefreshAccountRepo{}
			svc := NewTokenRefreshService(repo, nil, nil, nil, nil, nil, nil, &config.Config{}, nil)
			svc.SetPrivacyDeps(nil, &mockProxyRepoForOAuth{getByIDFunc: lookup})
			svc.ensureAntigravityModelCatalog(context.Background(), &Account{
				ID:          43,
				Platform:    PlatformAntigravity,
				Type:        AccountTypeOAuth,
				ProxyID:     &proxyID,
				Credentials: map[string]any{"access_token": "ag-token", AntigravityModelMappingModeCredentialKey: AntigravityModelMappingModeLive},
			})
			require.Zero(t, hits.Load(), "a bound proxy that cannot be loaded must not fall back to a direct connection")
			require.Zero(t, repo.updateExtraCalls)
		})
	}

	t.Run("no proxy repository", func(t *testing.T) {
		repo := &tokenRefreshAccountRepo{}
		svc := NewTokenRefreshService(repo, nil, nil, nil, nil, nil, nil, &config.Config{}, nil)
		svc.ensureAntigravityModelCatalog(context.Background(), &Account{
			ID:          44,
			Platform:    PlatformAntigravity,
			Type:        AccountTypeOAuth,
			ProxyID:     &proxyID,
			Credentials: map[string]any{"access_token": "ag-token", AntigravityModelMappingModeCredentialKey: AntigravityModelMappingModeLive},
		})
		require.Zero(t, hits.Load())
		require.Zero(t, repo.updateExtraCalls)
	})
}

func TestAntigravityQuotaProxyURLNeverFallsBackToDirect(t *testing.T) {
	proxyID := int64(9)
	account := &Account{ID: 45, Platform: PlatformAntigravity, ProxyID: &proxyID}

	_, err := NewAntigravityQuotaFetcher(&mockProxyRepoForOAuth{getByIDFunc: func(context.Context, int64) (*Proxy, error) {
		return nil, errors.New("db down")
	}}, &config.Config{}).GetProxyURL(context.Background(), account)
	require.Error(t, err)

	_, err = NewAntigravityQuotaFetcher(nil, &config.Config{}).GetProxyURL(context.Background(), account)
	require.Error(t, err)

	url, err := NewAntigravityQuotaFetcher(nil, &config.Config{}).GetProxyURL(context.Background(), &Account{ID: 46, Platform: PlatformAntigravity})
	require.NoError(t, err)
	require.Empty(t, url)
}

func TestFetchQuotaReturnsAntigravityModelCatalog(t *testing.T) {
	serveAntigravityCatalog(t, http.StatusOK)
	result, err := NewAntigravityQuotaFetcher(nil, &config.Config{}).FetchQuota(context.Background(), &Account{
		Platform:    PlatformAntigravity,
		Type:        AccountTypeOAuth,
		Credentials: map[string]any{"access_token": "token", "project_id": "project"},
	}, "")
	require.NoError(t, err)
	require.NotNil(t, result.AntigravityModelCatalog)
	require.Equal(t, normalizeAntigravityCatalogIDs(antigravityUltraCatalogFixture), result.AntigravityModelCatalog.Models)
}
