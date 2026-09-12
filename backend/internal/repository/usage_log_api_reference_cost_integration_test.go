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

func TestUsageLogAPIReferenceCostPersistence(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	repo := newUsageLogRepositoryWithSQL(client, integrationDB)
	user := mustCreateUser(t, client, &service.User{Email: "reference-" + uuid.NewString() + "@example.com"})
	key := mustCreateApiKey(t, client, &service.APIKey{UserID: user.ID, Key: "sk-reference-" + uuid.NewString(), Name: "reference"})
	account := mustCreateAccount(t, client, &service.Account{Name: "reference-" + uuid.NewString()})
	at := time.Now().UTC().Truncate(time.Second)
	amount := 1.2345678901
	log := &service.UsageLog{
		UserID: user.ID, APIKeyID: key.ID, AccountID: account.ID, RequestID: uuid.NewString(), Model: "gpt-5.6-terra", CreatedAt: at,
		APIReferenceCost: &amount, APIReferencePricing: &service.APIReferencePricingSnapshot{SchemaVersion: 1, Model: "gpt-5.6-terra", Source: "model_catalog", PricingAt: at},
	}
	inserted, err := repo.Create(ctx, log)
	require.NoError(t, err)
	require.True(t, inserted)
	got, err := repo.GetByID(ctx, log.ID)
	require.NoError(t, err)
	require.NotNil(t, got.APIReferenceCost)
	require.InDelta(t, amount, *got.APIReferenceCost, 1e-10)
	require.Equal(t, log.APIReferencePricing, got.APIReferencePricing)

	// Duplicate delivery cannot replace the original immutable price snapshot.
	changed := 100.0
	log.APIReferenceCost = &changed
	log.APIReferencePricing.Source = "changed"
	inserted, err = repo.Create(ctx, log)
	require.NoError(t, err)
	require.False(t, inserted)
	got, err = repo.GetByID(ctx, log.ID)
	require.NoError(t, err)
	require.InDelta(t, amount, *got.APIReferenceCost, 1e-10)
	require.Equal(t, "model_catalog", got.APIReferencePricing.Source)

	log.RequestID = uuid.NewString()
	log.APIReferenceCost, log.APIReferencePricing = nil, nil
	_, err = repo.Create(ctx, log)
	require.NoError(t, err)
	got, err = repo.GetByID(ctx, log.ID)
	require.NoError(t, err)
	require.Nil(t, got.APIReferenceCost)
	require.Nil(t, got.APIReferencePricing)
}
