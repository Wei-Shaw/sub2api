package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/cursor"
	gocache "github.com/patrickmn/go-cache"
	"github.com/stretchr/testify/require"
)

func TestGetAvailableModels_CursorUsesLivePickerCatalog(t *testing.T) {
	groupID := int64(51)
	var fetches int
	svc := NewGatewayService(
		&gatewayModelsRepo{byGroup: map[int64][]Account{
			groupID: {{
				ID:       7,
				Platform: PlatformCursor,
				Credentials: map[string]any{
					"access_token": "tok",
				},
			}},
		}},
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
	)
	svc.modelsListCache = gocache.New(time.Minute, time.Minute)
	svc.modelsListCacheTTL = time.Minute
	svc.cursorAvailableModels = func(ctx context.Context, creds cursor.Credentials) ([]cursor.AvailableModel, error) {
		fetches++
		require.Equal(t, "tok", creds.AccessToken)
		return []cursor.AvailableModel{
			{Name: "default", DisplayName: "Auto"},
			{Name: "claude-opus-5", DisplayName: "Claude Opus 5"},
			{Name: "gpt-5.6-sol", DisplayName: "GPT-5.6 Sol"},
		}, nil
	}

	got := svc.GetAvailableModels(context.Background(), &groupID, PlatformCursor)
	require.Equal(t, []string{"default", "claude-opus-5", "gpt-5.6-sol"}, got)
	require.Equal(t, 1, fetches)

	got2 := svc.GetAvailableModels(context.Background(), &groupID, PlatformCursor)
	require.Equal(t, got, got2)
	require.Equal(t, 1, fetches, "live catalog should be cached")
}

func TestGetAvailableModels_CursorMappingOverridesLiveCatalog(t *testing.T) {
	groupID := int64(52)
	svc := NewGatewayService(
		&gatewayModelsRepo{byGroup: map[int64][]Account{
			groupID: {{
				ID:       8,
				Platform: PlatformCursor,
				Credentials: map[string]any{
					"access_token": "tok",
					"model_mapping": map[string]any{
						"claude-opus-5": "claude-opus-5",
					},
				},
			}},
		}},
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
	)
	svc.cursorAvailableModels = func(context.Context, cursor.Credentials) ([]cursor.AvailableModel, error) {
		t.Fatal("live fetch should not run when mapping is set")
		return nil, nil
	}

	got := svc.GetAvailableModels(context.Background(), &groupID, PlatformCursor)
	require.Equal(t, []string{"claude-opus-5"}, got)
}

func TestNormalizeCursorAccessToken(t *testing.T) {
	require.Equal(t, "eyJhbGciOiJIUzI1NiJ9", normalizeCursorAccessToken("user_01ABC::eyJhbGciOiJIUzI1NiJ9"))
	require.Equal(t, "plain", normalizeCursorAccessToken("  plain  "))
}

func TestResolveCursorChatModelWarnsOnAliasFallback(t *testing.T) {
	upstream, warnings := resolveCursorChatModel("opus", cursor.RunOpts{})
	require.Equal(t, "claude-opus-5-medium", upstream)
	require.Equal(t, "model_fallback", warnings[0]["code"])
	require.Equal(t, "model_variant", warnings[1]["code"])
	require.Contains(t, warnings[0]["message"], "claude-opus-5")

	upstream, warnings = resolveCursorChatModel("claude-opus-5", cursor.RunOpts{})
	require.Equal(t, "claude-opus-5-medium", upstream)
	require.Len(t, warnings, 1)
	require.Equal(t, "model_variant", warnings[0]["code"])

	upstream, warnings = resolveCursorChatModel("grok-4.6", cursor.RunOpts{})
	require.Equal(t, "cursor-grok-4.6-medium", upstream)
	require.Equal(t, "model_variant", warnings[0]["code"])

	upstream, warnings = resolveCursorChatModel("composer-2.5", cursor.RunOpts{})
	require.Equal(t, "composer-2.5", upstream)
	require.Empty(t, warnings)

	upstream, warnings = resolveCursorChatModel("cursor-grok-4.6-medium", cursor.RunOpts{})
	require.Equal(t, "cursor-grok-4.6-medium", upstream)
	require.Empty(t, warnings)
}

type gatewayModelsRepo struct {
	AccountRepository
	byGroup map[int64][]Account
}

func (r *gatewayModelsRepo) ListSchedulableByGroupID(_ context.Context, groupID int64) ([]Account, error) {
	out := r.byGroup[groupID]
	cp := make([]Account, len(out))
	copy(cp, out)
	return cp, nil
}
