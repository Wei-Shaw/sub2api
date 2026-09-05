package repository

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestAPIKeyRepositoryPersistsMaxRateMultiplier(t *testing.T) {
	repo, client := newAPIKeyRepoSQLite(t)
	ctx := context.Background()
	user := mustCreateAPIKeyRepoUser(t, ctx, client, "max-rate-multiplier@test.com")
	max := 2.75
	key := &service.APIKey{
		UserID:            user.ID,
		Key:               "sk-max-rate-multiplier-unit",
		Name:              "Max rate multiplier",
		Status:            service.StatusActive,
		MaxRateMultiplier: &max,
	}

	require.NoError(t, repo.Create(ctx, key))
	got, err := repo.GetByID(ctx, key.ID)
	require.NoError(t, err)
	require.NotNil(t, got.MaxRateMultiplier)
	require.Equal(t, max, *got.MaxRateMultiplier)

	updated := 1.5
	key.MaxRateMultiplier = &updated
	require.NoError(t, repo.Update(ctx, key, service.APIKeyUpdateFields{MaxRateMultiplier: true}))
	got, err = repo.GetByKeyForAuth(ctx, key.Key)
	require.NoError(t, err)
	require.NotNil(t, got.MaxRateMultiplier)
	require.Equal(t, updated, *got.MaxRateMultiplier)

	key.MaxRateMultiplier = nil
	require.NoError(t, repo.Update(ctx, key, service.APIKeyUpdateFields{MaxRateMultiplier: true}))
	got, err = repo.GetByID(ctx, key.ID)
	require.NoError(t, err)
	require.Nil(t, got.MaxRateMultiplier)
}
