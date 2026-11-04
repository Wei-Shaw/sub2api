package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/minimax"
	gocache "github.com/patrickmn/go-cache"
	"github.com/stretchr/testify/require"
)

func TestMiniMaxMediaCatalogKeepsCallerDefaultsOutsideCache(t *testing.T) {
	for _, ordinaryFirst := range []bool{true, false} {
		name := "codex_first"
		if ordinaryFirst {
			name = "ordinary_first"
		}
		t.Run(name, func(t *testing.T) {
			group := &Group{ID: 1, Platform: PlatformMiniMax}
			repo := &modelsListAccountRepoStub{byGroup: map[int64][]Account{1: {{
				Platform: PlatformMiniMax, Type: AccountTypeAPIKey,
				Credentials: map[string]any{"api_key": "test"},
				Extra:       map[string]any{"minimax_media_enabled": true, "minimax_media_base_url": "https://api.minimax.io"},
			}}}}
			s := &GatewayService{accountRepo: repo, groupRepo: &mediaCatalogGroup{group: group},
				modelsListCache: gocache.New(time.Minute, time.Minute), modelsListCacheTTL: time.Minute}
			ordinary := []string{"ordinary-text", minimax.Image}
			codex := []string{"codex-text"}
			first, second := ordinary, codex
			if !ordinaryFirst {
				first, second = codex, ordinary
			}
			ctx := context.Background()
			firstModels := s.GetAvailableModels(ctx, &group.ID, PlatformMiniMax, first)
			require.Contains(t, firstModels, first[0])
			require.Contains(t, firstModels, minimax.SpeechHD)
			firstModels[0] = "caller-mutation"
			secondModels := s.GetAvailableModels(ctx, &group.ID, PlatformMiniMax, second)
			require.Contains(t, secondModels, second[0])
			require.NotContains(t, secondModels, first[0])
			require.NotContains(t, secondModels, "caller-mutation")
			require.Contains(t, secondModels, minimax.Image)
			count := 0
			for _, model := range secondModels {
				if model == minimax.Image {
					count++
				}
			}
			require.Equal(t, 1, count, "defaults and media must be deduplicated")
			mediaOnly := s.GetAvailableModels(ctx, &group.ID, PlatformMiniMax)
			require.Contains(t, mediaOnly, minimax.H3)
			require.NotContains(t, mediaOnly, ordinary[0])
			require.NotContains(t, mediaOnly, codex[0])
			require.EqualValues(t, 1, repo.listByGroupCalls.Load(), "all callers share the underlying catalog cache")
		})
	}
}

func TestMiniMaxMediaCatalogRespectsMappingsCapabilityAndCompositeDefaults(t *testing.T) {
	group := &Group{ID: 1, Platform: PlatformMiniMax}
	account := Account{Platform: PlatformMiniMax, Type: AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "test", "model_mapping": map[string]any{"configured-model": "MiniMax-M3"}},
		Extra:       map[string]any{"minimax_media_enabled": true, "minimax_media_base_url": "https://api.minimax.io"}}
	repo := &mediaCatalogAccounts{accounts: []Account{account}}
	s := &GatewayService{accountRepo: repo, groupRepo: &mediaCatalogGroup{group: group},
		modelsListCache: gocache.New(time.Minute, time.Minute), modelsListCacheTTL: time.Minute}
	ctx := context.Background()
	models := s.GetAvailableModels(ctx, &group.ID, PlatformMiniMax, []string{"default-text"})
	require.Contains(t, models, "configured-model")
	require.Contains(t, models, minimax.Image)
	require.NotContains(t, models, "default-text", "an explicit account catalog remains authoritative")

	account.Extra["minimax_media_enabled"] = false
	s.InvalidateAvailableModelsCache(&group.ID, PlatformMiniMax)
	require.Equal(t, []string{"configured-model"}, s.GetAvailableModels(ctx, &group.ID, PlatformMiniMax, []string{"default-text"}))

	delete(account.Credentials, "model_mapping")
	s.InvalidateAvailableModelsCache(&group.ID, PlatformMiniMax)
	require.Nil(t, s.GetAvailableModels(ctx, &group.ID, PlatformMiniMax, []string{"default-text"}), "disabled media preserves the existing nil/default behavior")

	account.Extra["minimax_media_enabled"] = true
	group.Platform = PlatformComposite
	s.InvalidateAvailableModelsCache(&group.ID, PlatformMiniMax)
	models = s.GetAvailableModels(ctx, &group.ID, PlatformMiniMax)
	require.Contains(t, models, minimax.Image)
	require.NotContains(t, models, "default-text", "composite callers do not advertise unrelated platform defaults")
}
