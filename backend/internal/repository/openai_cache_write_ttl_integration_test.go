//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestUsageLogRepositoryCacheWriteTTLRoundTrip(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	repo := newUsageLogRepositoryWithSQL(client, integrationDB)
	user := mustCreateUser(t, client, &service.User{Email: uuid.NewString() + "@example.com"})
	key := mustCreateApiKey(t, client, &service.APIKey{UserID: user.ID, Key: "synthetic-" + uuid.NewString(), Name: "ttl"})
	account := mustCreateAccount(t, client, &service.Account{Name: "ttl-" + uuid.NewString()})
	t.Cleanup(func() {
		// These committed rows must not leak into later dashboard/global totals.
		for _, row := range []struct {
			table string
			id    int64
		}{{"usage_logs", key.ID}, {"api_keys", key.ID}, {"accounts", account.ID}, {"users", user.ID}} {
			column := "id"
			if row.table == "usage_logs" {
				column = "api_key_id"
			}
			_, err := integrationDB.ExecContext(ctx, "DELETE FROM "+row.table+" WHERE "+column+" = $1", row.id)
			require.NoError(t, err)
		}
	})
	for _, mode := range []string{"single", "batch", "best_effort"} {
		t.Run(mode, func(t *testing.T) {
			log := &service.UsageLog{
				UserID: user.ID, APIKeyID: key.ID, AccountID: account.ID,
				RequestID: uuid.NewString(), Model: "gpt-6-luna", InputTokens: 3,
				CacheCreationTokens: 2399, CacheCreation30mTokens: 9, CacheCreation12hTokens: 2390,
				CreatedAt: time.Now().UTC(),
			}
			switch mode {
			case "single":
				inserted, err := repo.createSingle(ctx, integrationDB, log)
				require.NoError(t, err)
				require.True(t, inserted)
			case "batch":
				inserted, err := repo.Create(ctx, log)
				require.NoError(t, err)
				require.True(t, inserted)
			case "best_effort":
				require.NoError(t, repo.CreateBestEffort(ctx, log))
				require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT id FROM usage_logs WHERE request_id = $1 AND api_key_id = $2", log.RequestID, key.ID).Scan(&log.ID))
			}
			stored, err := repo.GetByID(ctx, log.ID)
			require.NoError(t, err)
			require.Equal(t, 2399, stored.CacheCreationTokens)
			require.Equal(t, 9, stored.CacheCreation30mTokens)
			require.Equal(t, 2390, stored.CacheCreation12hTokens)
			require.Zero(t, stored.CacheCreation5mTokens)
			require.Zero(t, stored.CacheCreation1hTokens)
		})
	}
}

func TestChannelCacheWriteTTLPricesRoundTrip(t *testing.T) {
	ctx := context.Background()
	short, long, statsLong, intervalLong := 0.8125e-6, 1.625e-6, 0.75e-6, 3.25e-6
	channel := &service.Channel{
		Name: t.Name(), Status: service.StatusActive, BillingModelSource: service.BillingModelSourceRequested,
		ModelPricing: []service.ChannelModelPricing{{
			Platform: service.PlatformOpenAI, Models: []string{"gpt-6-luna"}, BillingMode: service.BillingModeToken,
			CacheWritePrice: &short, CacheWrite12hPrice: &long,
			Intervals: []service.PricingInterval{{MinTokens: 100, CacheWrite12hPrice: &intervalLong}},
		}},
		AccountStatsPricingRules: []service.AccountStatsPricingRule{{
			Name: "TTL accounting", GroupIDs: []int64{}, AccountIDs: []int64{}, Pricing: []service.ChannelModelPricing{{
				Platform: service.PlatformOpenAI, Models: []string{"gpt-6-luna"}, BillingMode: service.BillingModeToken,
				CacheWritePrice: &short, CacheWrite12hPrice: &statsLong,
				Intervals: []service.PricingInterval{{MinTokens: 100, CacheWrite12hPrice: &intervalLong}},
			}},
		}},
	}
	repo := &channelRepository{db: integrationDB}
	require.NoError(t, repo.Create(ctx, channel))
	t.Cleanup(func() { require.NoError(t, repo.Delete(ctx, channel.ID)) })
	loaded, err := repo.GetByID(ctx, channel.ID)
	require.NoError(t, err)
	require.InDelta(t, long, *loaded.ModelPricing[0].CacheWrite12hPrice, 1e-15)
	require.InDelta(t, intervalLong, *loaded.ModelPricing[0].Intervals[0].CacheWrite12hPrice, 1e-15)
	require.InDelta(t, statsLong, *loaded.AccountStatsPricingRules[0].Pricing[0].CacheWrite12hPrice, 1e-15)
	require.InDelta(t, intervalLong, *loaded.AccountStatsPricingRules[0].Pricing[0].Intervals[0].CacheWrite12hPrice, 1e-15)
	batch, err := repo.batchLoadModelPricing(ctx, []int64{channel.ID})
	require.NoError(t, err)
	require.InDelta(t, long, *batch[channel.ID][0].CacheWrite12hPrice, 1e-15)
	stats, err := repo.batchLoadAccountStatsPricingRules(ctx, []int64{channel.ID})
	require.NoError(t, err)
	require.InDelta(t, statsLong, *stats[channel.ID][0].Pricing[0].CacheWrite12hPrice, 1e-15)

	zero := 0.0
	pricing := &loaded.ModelPricing[0]
	pricing.CacheWrite12hPrice = &zero
	require.NoError(t, repo.UpdateModelPricing(ctx, pricing))
	listed, err := repo.ListModelPricing(ctx, channel.ID)
	require.NoError(t, err)
	require.NotNil(t, listed[0].CacheWrite12hPrice)
	require.Zero(t, *listed[0].CacheWrite12hPrice)

	pricing.CacheWrite12hPrice = nil
	require.NoError(t, repo.ReplaceModelPricing(ctx, channel.ID, []service.ChannelModelPricing{*pricing}))
	listed, err = repo.ListModelPricing(ctx, channel.ID)
	require.NoError(t, err)
	require.Nil(t, listed[0].CacheWrite12hPrice)
	require.InDelta(t, intervalLong, *listed[0].Intervals[0].CacheWrite12hPrice, 1e-15)
}
