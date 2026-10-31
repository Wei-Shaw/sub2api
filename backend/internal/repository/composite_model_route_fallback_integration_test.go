//go:build integration

package repository

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	dbmigrations "github.com/Wei-Shaw/sub2api/migrations"
	"github.com/stretchr/testify/require"
)

func TestMigration246CompositeFallbackTargets(t *testing.T) {
	ctx := context.Background()
	tx := testTx(t)
	_, err := tx.ExecContext(ctx, `ALTER TABLE composite_model_routes DROP COLUMN fallback_targets`)
	require.NoError(t, err)

	var groupID, legacyID int64
	require.NoError(t, tx.QueryRowContext(ctx, `INSERT INTO groups (name, platform) VALUES ('migration-246', 'composite') RETURNING id`).Scan(&groupID))
	require.NoError(t, tx.QueryRowContext(ctx, `
INSERT INTO composite_model_routes
    (group_id, public_model, match_type, target_platform, upstream_model, endpoint, priority, enabled, notes)
VALUES ($1, 'legacy-flash', 'exact', 'opencode_go', 'deepseek-v4.1-flash', 'messages', 17, false, 'legacy route')
RETURNING id`, groupID).Scan(&legacyID))
	var legacyBefore string
	require.NoError(t, tx.QueryRowContext(ctx, `SELECT to_jsonb(r)::text FROM composite_model_routes r WHERE id=$1`, legacyID).Scan(&legacyBefore))

	migrationSQL, err := dbmigrations.FS.ReadFile("246_composite_route_fallback_targets.sql")
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, string(migrationSQL))
	require.NoError(t, err)
	requireColumn(t, tx, "composite_model_routes", "fallback_targets", "jsonb", 0, false)
	requireColumnDefaultContains(t, tx, "composite_model_routes", "fallback_targets", "'[]'::jsonb")

	var legacyAfter, legacyTargets string
	require.NoError(t, tx.QueryRowContext(ctx, `
SELECT (to_jsonb(r) - 'fallback_targets')::text, fallback_targets::text
FROM composite_model_routes r WHERE id=$1`, legacyID).Scan(&legacyAfter, &legacyTargets))
	require.JSONEq(t, legacyBefore, legacyAfter)
	require.JSONEq(t, `[]`, legacyTargets)

	var defaultTargets string
	require.NoError(t, tx.QueryRowContext(ctx, `
INSERT INTO composite_model_routes (group_id, public_model, target_platform)
VALUES ($1, 'new-default', 'deepseek') RETURNING fallback_targets::text`, groupID).Scan(&defaultTargets))
	require.JSONEq(t, `[]`, defaultTargets)

	const targets = `[{"target_platform":"deepseek","upstream_model":"deepseek-flash"}]`
	_, err = tx.ExecContext(ctx, `UPDATE composite_model_routes SET fallback_targets=$1::jsonb WHERE id=$2`, targets, legacyID)
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, string(migrationSQL))
	require.NoError(t, err)
	require.NoError(t, tx.QueryRowContext(ctx, `SELECT fallback_targets::text FROM composite_model_routes WHERE id=$1`, legacyID).Scan(&legacyTargets))
	require.JSONEq(t, targets, legacyTargets, "migration replay must preserve configured targets")
}

func TestCompositeRouteFallbackTargetsRepositoryRoundTrip(t *testing.T) {
	ctx := context.Background()
	client := testEntTx(t).Client()
	group := mustCreateGroup(t, client, &service.Group{Name: "fallback-persistence", Platform: service.PlatformComposite})
	repo := NewCompositeModelRouteRepository(client)
	targets := []service.CompositeFallbackTarget{{TargetPlatform: service.PlatformDeepseek, UpstreamModel: "deepseek-flash"}}

	newRoute := func(model string, fallback []service.CompositeFallbackTarget) *service.CompositeModelRoute {
		return &service.CompositeModelRoute{
			GroupID: group.ID, PublicModel: model, MatchType: service.CompositeRouteMatchExact,
			TargetPlatform: service.PlatformOpenCodeGo, UpstreamModel: "deepseek-v4.1-flash",
			FallbackTargets: fallback, Endpoint: service.CompositeRouteEndpointAny, Priority: 100, Enabled: true,
		}
	}
	readTargets := func(routeID int64, want []service.CompositeFallbackTarget) {
		t.Helper()
		row, err := client.CompositeModelRoute.Get(ctx, routeID)
		require.NoError(t, err)
		require.NotNil(t, row.FallbackTargets, "SQL JSON must be an array, not null")
		require.Equal(t, want, row.FallbackTargets)
		routes, err := repo.ListByGroup(ctx, group.ID, true)
		require.NoError(t, err)
		for _, route := range routes {
			if route.ID == routeID {
				require.Equal(t, want, route.FallbackTargets)
				encoded, err := json.Marshal(route)
				require.NoError(t, err)
				var decoded struct {
					FallbackTargets []service.CompositeFallbackTarget `json:"fallback_targets"`
				}
				require.NoError(t, json.Unmarshal(encoded, &decoded))
				require.NotNil(t, decoded.FallbackTargets)
				require.Equal(t, want, decoded.FallbackTargets)
				return
			}
		}
		t.Fatalf("route %d not found", routeID)
	}

	empty := []service.CompositeFallbackTarget{}
	legacy := newRoute("legacy-empty", nil)
	require.NoError(t, repo.Create(ctx, legacy))
	readTargets(legacy.ID, empty)

	route := newRoute("paid-flash", targets)
	require.NoError(t, repo.Create(ctx, route))
	readTargets(route.ID, targets)

	legacy.FallbackTargets = targets
	require.NoError(t, repo.Update(ctx, legacy))
	readTargets(legacy.ID, targets)

	for _, cleared := range [][]service.CompositeFallbackTarget{empty, nil} {
		route.FallbackTargets = targets
		require.NoError(t, repo.Update(ctx, route))
		readTargets(route.ID, targets)
		route.FallbackTargets = cleared
		require.NoError(t, repo.Update(ctx, route))
		readTargets(route.ID, empty)
	}
}
