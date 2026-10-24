package repository

import (
	"encoding/json"
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestCompositeRouteFallbackEntityConversion(t *testing.T) {
	row := &dbent.CompositeModelRoute{ID: 1, GroupID: 101, FallbackTargets: []domain.CompositeFallbackTarget{{TargetPlatform: domain.PlatformDeepseek, UpstreamModel: "deepseek-flash"}}}
	route := compositeModelRouteEntityToService(row)
	require.Equal(t, row.FallbackTargets, route.FallbackTargets)
	route.FallbackTargets[0].UpstreamModel = "changed"
	require.Equal(t, "deepseek-flash", row.FallbackTargets[0].UpstreamModel)
	empty := compositeModelRouteEntityToService(&dbent.CompositeModelRoute{})
	encoded, err := json.Marshal(empty)
	require.NoError(t, err)
	require.Contains(t, string(encoded), `"fallback_targets":[]`)
}
