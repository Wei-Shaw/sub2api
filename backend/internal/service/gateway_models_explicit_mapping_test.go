//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	gocache "github.com/patrickmn/go-cache"
	"github.com/stretchr/testify/require"
)

func TestGetAvailableModels_AntigravityExplicitCatalog(t *testing.T) {
	for _, platform := range []string{PlatformAntigravity, PlatformGemini, ""} {
		t.Run(platform, func(t *testing.T) {
			mapping := map[string]any{
				"gemini-3-flash":   "gemini-3-flash",
				"gemini-custom":    "gemini-3.8-flash-high",
				"gemini-3.6-flash": "gemini-3.6-flash",
			}
			account := Account{
				ID: 1, Platform: PlatformAntigravity,
				Credentials: map[string]any{"model_mapping": mapping},
				Extra:       map[string]any{"mixed_scheduling": true},
			}
			groupID := int64(10)
			repo := &modelsListAccountRepoStub{byGroup: map[int64][]Account{groupID: {account}}}
			svc := &GatewayService{
				accountRepo:        repo,
				modelsListCache:    gocache.New(time.Minute, time.Minute),
				modelsListCacheTTL: time.Minute,
			}
			require.ElementsMatch(t, []string{"gemini-3-flash", "gemini-custom", "gemini-3.6-flash"},
				svc.GetAvailableModels(context.Background(), &groupID, platform))

			// Reproduce deleting a default passthrough in the account editor.
			delete(mapping, "gemini-3.6-flash")
			svc.InvalidateAvailableModelsCache(&groupID, platform)
			require.ElementsMatch(t, []string{"gemini-3-flash", "gemini-custom"},
				svc.GetAvailableModels(context.Background(), &groupID, platform))
			require.True(t, account.IsModelSupported("gemini-3.6-flash"),
				"catalog cleanup must not change legacy direct-request routing")
		})
	}
}

func TestGetAvailableModels_AntigravityDefaultsWithoutExplicitMapping(t *testing.T) {
	for _, tc := range []struct {
		name        string
		credentials map[string]any
	}{
		{name: "nil credentials"},
		{name: "missing mapping", credentials: map[string]any{}},
		{name: "empty mapping", credentials: map[string]any{"model_mapping": map[string]any{}}},
		{name: "invalid mapping", credentials: map[string]any{"model_mapping": map[string]any{"invalid": 123}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			account := Account{ID: 1, Platform: PlatformAntigravity, Credentials: tc.credentials}
			repo := &modelsListAccountRepoStub{all: []Account{account}}
			svc := &GatewayService{accountRepo: repo}
			models := svc.GetAvailableModels(context.Background(), nil, PlatformAntigravity)
			require.Len(t, models, len(domain.DefaultAntigravityModelMapping))
			for model := range domain.DefaultAntigravityModelMapping {
				require.Contains(t, models, model)
			}
		})
	}
}

func TestGetAvailableModels_AntigravityKeepsExplicitWildcard(t *testing.T) {
	account := Account{
		ID: 1, Platform: PlatformAntigravity,
		Credentials: map[string]any{"model_mapping": map[string]any{
			"gemini-3.8-*":  "gemini-3.8-flash-high",
			"claude-custom": "claude-sonnet-4-6",
		}},
	}
	repo := &modelsListAccountRepoStub{all: []Account{account}}
	svc := &GatewayService{accountRepo: repo}
	require.ElementsMatch(t, []string{"gemini-3.8-*", "claude-custom"},
		svc.GetAvailableModels(context.Background(), nil, PlatformAntigravity))
}

func TestGetAvailableModels_AntigravityFilteredEmptyCatalogIsCached(t *testing.T) {
	groupID := int64(11)
	mapping := map[string]any{
		"gemini-custom": "gemini-3.8-flash-high",
		"claude-custom": "claude-sonnet-4-6",
	}
	repo := &modelsListAccountRepoStub{byGroup: map[int64][]Account{groupID: {{
		ID: 1, Platform: PlatformAntigravity,
		Credentials: map[string]any{"model_mapping": mapping},
		Extra:       map[string]any{"mixed_scheduling": true},
	}}}}
	svc := &GatewayService{
		accountRepo:        repo,
		modelsListCache:    gocache.New(time.Minute, time.Minute),
		modelsListCacheTTL: time.Minute,
	}
	require.Equal(t, []string{"gemini-custom"}, svc.GetAvailableModels(context.Background(), &groupID, PlatformGemini))
	delete(mapping, "gemini-custom")
	svc.InvalidateAvailableModelsCache(&groupID, PlatformGemini)
	for range 2 {
		models := svc.GetAvailableModels(context.Background(), &groupID, PlatformGemini)
		require.NotNil(t, models)
		require.Empty(t, models)
	}
	require.Equal(t, int64(2), repo.listByGroupCalls.Load(), "the second empty read must hit the cache")
}
