package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/modelcatalog"
	"github.com/stretchr/testify/require"
)

func TestListCatalogStorefrontModels_IncludesGemini37(t *testing.T) {
	models := ListCatalogStorefrontModels("antigravity")
	require.NotEmpty(t, models)

	var flash37 *CatalogStorefrontModel
	for i := range models {
		if models[i].ID == "gemini-3.7-flash" {
			flash37 = &models[i]
			break
		}
	}
	require.NotNil(t, flash37)
	require.Equal(t, "gemini-3.7-flash", flash37.CanonicalID)
	require.NotNil(t, flash37.InputPrice)
	require.InDelta(t, 0.75e-6, *flash37.InputPrice, 1e-12)
	require.True(t, modelcatalog.Locked("gemini-3.7-flash"))
}

func TestAnnotateCatalogStorefrontCoverage_CountsSnapshotsNotIntersection(t *testing.T) {
	parentID := int64(8)
	accounts := []Account{
		{
			ID:       1,
			Platform: PlatformAntigravity,
			Extra:    ApplyUpstreamModelSnapshot(nil, []string{"gemini-3.7-flash", "gemini-3.6-flash-tiered"}, time.Unix(1, 0).UTC()),
		},
		{
			ID:       2,
			Platform: PlatformAntigravity,
			Extra:    ApplyUpstreamModelSnapshot(nil, []string{"gemini-3.6-flash-tiered"}, time.Unix(1, 0).UTC()),
			Credentials: map[string]any{
				"model_mapping": map[string]any{"gemini-3.6-flash": "gemini-3.6-flash-tiered"},
			},
		},
		{ID: 3, Platform: PlatformAntigravity},
		{ID: 4, Platform: PlatformOpenAI, Extra: ApplyUpstreamModelSnapshot(nil, []string{"gpt-5.4"}, time.Unix(1, 0).UTC())},
		{ID: 5, Platform: PlatformAntigravity, ParentAccountID: &parentID},
	}
	filtered := filterStorefrontCoverageAccounts(accounts, "antigravity")
	require.Equal(t, []int64{1, 2, 3}, []int64{filtered[0].ID, filtered[1].ID, filtered[2].ID})

	models := AnnotateCatalogStorefrontCoverage([]CatalogStorefrontModel{
		{ID: "gemini-3.7-flash"},
		{ID: "gemini-3.6-flash"},
		{ID: "gemini-3.6-flash-tiered"},
		{ID: "claude-unknown"},
	}, filtered)
	require.Equal(t, 1, *models[0].CoverageHave)
	require.Equal(t, 3, *models[0].CoverageTotal)
	require.Equal(t, 2, *models[0].CoverageSynced)
	require.Equal(t, 1, *models[1].CoverageHave)
	require.Equal(t, 2, *models[2].CoverageHave)
	require.Equal(t, 0, *models[3].CoverageHave)
}

func TestListCatalogStorefrontModelsWithCoverage_WithoutGroupsKeepsPlainCatalog(t *testing.T) {
	svc := &ChannelService{}
	models := svc.ListCatalogStorefrontModelsWithCoverage(context.Background(), "antigravity", nil)
	require.NotEmpty(t, models)
	require.Nil(t, models[0].CoverageTotal)
}
