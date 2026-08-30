//go:build integration

package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestDisplayPricingRepositoryDeleteProviderCascadesAndReportsModelCount(t *testing.T) {
	ctx := context.Background()
	providerKey := fmt.Sprintf("cascade_%d", time.Now().UnixNano())
	repo := NewDisplayPricingRepository(integrationDB)
	require.NoError(t, repo.CreateProvider(ctx, &service.DisplayPricingProvider{
		Provider: providerKey, DisplayName: "Cascade test", Currency: "USD", LogoKey: "openai",
	}))
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(ctx, `DELETE FROM display_pricing_providers WHERE provider=$1`, providerKey)
	})

	for i := 0; i < 2; i++ {
		_, err := integrationDB.ExecContext(ctx, `
			INSERT INTO display_model_prices (platform, model_name, provider, billing_mode, currency)
			VALUES ('openai', $1, $2, 'token', 'USD')`, fmt.Sprintf("cascade-model-%d", i), providerKey)
		require.NoError(t, err)
	}

	deletedModels, err := repo.DeleteProvider(ctx, providerKey)
	require.NoError(t, err)
	require.EqualValues(t, 2, deletedModels)

	var providerCount, modelCount int
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM display_pricing_providers WHERE provider=$1`, providerKey).Scan(&providerCount))
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM display_model_prices WHERE provider=$1`, providerKey).Scan(&modelCount))
	require.Zero(t, providerCount)
	require.Zero(t, modelCount)
}
