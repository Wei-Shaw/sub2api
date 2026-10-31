//go:build integration

package repository

import (
	"context"
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestBytePlusPlatformPersistence(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	client := tx.Client()
	txCtx := dbent.NewTxContext(ctx, tx)
	group := mustCreateGroup(t, client, &service.Group{Name: "BytePlus", Platform: service.PlatformBytePlus})
	account := mustCreateAccount(t, client, &service.Account{
		Name: "BytePlus", Platform: service.PlatformBytePlus, Type: service.AccountTypeAPIKey,
		Credentials: map[string]any{"base_url": service.DefaultBytePlusBaseURL, "openai_capabilities": []string{"chat_completions", "seedance"}},
	})
	require.Equal(t, service.PlatformBytePlus, group.Platform)
	require.Equal(t, service.PlatformBytePlus, account.Platform)
	userID := mustCreateUserForQuota(t, client)
	quotaRepo := NewUserPlatformQuotaRepository(client)
	dailyLimit := float64(5)
	require.NoError(t, quotaRepo.BulkInsertInitial(txCtx, []UserPlatformQuotaRecord{{UserID: userID, Platform: service.PlatformBytePlus, DailyLimitUSD: &dailyLimit}}))
	routeRepo := NewCompositeModelRouteRepository(client)
	composite := mustCreateGroup(t, client, &service.Group{Name: "Composite BytePlus", Platform: service.PlatformComposite})
	route := &service.CompositeModelRoute{GroupID: composite.ID, PublicModel: "seed", MatchType: "exact", TargetPlatform: service.PlatformBytePlus, UpstreamModel: "dola-seed-2-1-turbo-260628", Endpoint: "any", Priority: 100, Enabled: true}
	require.NoError(t, routeRepo.Create(txCtx, route))
	routes, err := routeRepo.ListByGroup(txCtx, composite.ID, true)
	require.NoError(t, err)
	require.Len(t, routes, 1)
	require.Equal(t, service.PlatformBytePlus, routes[0].TargetPlatform)
}
