//go:build integration

package repository

import (
	"context"
	"database/sql"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	dbmigrations "github.com/Wei-Shaw/sub2api/migrations"
	"github.com/stretchr/testify/require"
)

func TestChannelUltrafastPricingMigrationPreservesExistingCards(t *testing.T) {
	ctx := context.Background()
	tx := testTx(t)
	_, err := tx.ExecContext(ctx, `ALTER TABLE channel_model_pricing DROP COLUMN ultrafast_multiplier`)
	require.NoError(t, err)
	var channelID, pricingID int64
	require.NoError(t, tx.QueryRowContext(ctx, `INSERT INTO channels (name) VALUES ('ultrafast-migration') RETURNING id`).Scan(&channelID))
	require.NoError(t, tx.QueryRowContext(ctx, `INSERT INTO channel_model_pricing (channel_id, models, fast_multiplier) VALUES ($1, '["gpt-6-astra"]', 2.5) RETURNING id`, channelID).Scan(&pricingID))
	migration, err := dbmigrations.FS.ReadFile("252_channel_pricing_ultrafast_multiplier.sql")
	require.NoError(t, err)
	for range 2 {
		_, err = tx.ExecContext(ctx, string(migration))
		require.NoError(t, err)
		var ultrafast sql.NullFloat64
		var fast float64
		require.NoError(t, tx.QueryRowContext(ctx, `SELECT ultrafast_multiplier, fast_multiplier FROM channel_model_pricing WHERE id = $1`, pricingID).Scan(&ultrafast, &fast))
		require.False(t, ultrafast.Valid)
		require.Equal(t, 2.5, fast)
	}
	_, err = tx.ExecContext(ctx, `UPDATE channel_model_pricing SET ultrafast_multiplier = 1.25 WHERE id = $1`, pricingID)
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, string(migration))
	require.NoError(t, err)
	var configured float64
	require.NoError(t, tx.QueryRowContext(ctx, `SELECT ultrafast_multiplier FROM channel_model_pricing WHERE id = $1`, pricingID).Scan(&configured))
	require.Equal(t, 1.25, configured)
}

func TestChannelUltrafastPricingCreateUpdateReplace(t *testing.T) {
	ctx := context.Background()
	channel := &service.Channel{}
	repo := newReasoningPricingIntegrationChannel(t, channel)
	multiplier := 3.0
	pricing := service.ChannelModelPricing{
		ChannelID: channel.ID, Platform: service.PlatformOpenAI, Models: []string{"gpt-6-astra"},
		UltrafastMultiplier: &multiplier,
	}
	require.NoError(t, repo.CreateModelPricing(ctx, &pricing))
	assertStored := func(want *float64) {
		t.Helper()
		listed, err := repo.ListModelPricing(ctx, channel.ID)
		require.NoError(t, err)
		require.Len(t, listed, 1)
		require.Equal(t, want, listed[0].UltrafastMultiplier)
		batch, err := repo.batchLoadModelPricing(ctx, []int64{channel.ID})
		require.NoError(t, err)
		require.Len(t, batch[channel.ID], 1)
		require.Equal(t, want, batch[channel.ID][0].UltrafastMultiplier)
	}
	assertStored(&multiplier)
	multiplier = 1.25
	require.NoError(t, repo.UpdateModelPricing(ctx, &pricing))
	assertStored(&multiplier)
	pricing.UltrafastMultiplier = nil
	require.NoError(t, repo.UpdateModelPricing(ctx, &pricing))
	assertStored(nil)
	var stored sql.NullFloat64
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT ultrafast_multiplier FROM channel_model_pricing WHERE id = $1`, pricing.ID).Scan(&stored))
	require.False(t, stored.Valid)

	multiplier = 1
	pricing.UltrafastMultiplier = &multiplier
	require.NoError(t, repo.ReplaceModelPricing(ctx, channel.ID, []service.ChannelModelPricing{pricing}))
	assertStored(&multiplier)
	for _, invalid := range []float64{0, -1} {
		err := repo.ReplaceModelPricing(ctx, channel.ID, []service.ChannelModelPricing{{
			Platform: service.PlatformOpenAI, Models: []string{"invalid"}, UltrafastMultiplier: &invalid,
		}})
		require.Error(t, err)
		assertStored(&multiplier) // A rejected replacement must preserve the old card.
	}
}
