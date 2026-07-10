//go:build integration

package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func TestBillingTransactionRepositoryAppend_IdempotencyPrecisionAndSourceOrder(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	repo := NewBillingTransactionRepository(integrationDB)
	user := mustCreateUser(t, client, &service.User{
		Email:        fmt.Sprintf("transaction-%s@example.com", uuid.NewString()),
		PasswordHash: "hash",
		Balance:      100,
	})

	transaction := func(key, kind, amount string) *service.BillingTransaction {
		return &service.BillingTransaction{
			TransactionKey:   key,
			SourceType:       "video_task",
			SourceID:         501,
			TransactionKind:  kind,
			UserID:           user.ID,
			AmountOriginal:   service.MustUSD(amount),
			AmountUSD:        service.MustUSD(amount),
			ExchangeRate:     decimal.RequireFromString("1.0000000000"),
			ExchangeRateAsOf: time.Now().UTC(),
			PricingSource:    "provider_usage",
			PricingVersion:   "v1",
			BalanceBefore:    service.MustUSD("100"),
			BalanceAfter:     service.MustUSD("98.7654321099"),
			Metadata:         json.RawMessage(`{"usage":{"tokens":123}}`),
		}
	}

	key := "transaction-" + uuid.NewString()
	firstInput := transaction(key, "charge", "1.2345678901")
	first, err := repo.Append(ctx, firstInput)
	require.NoError(t, err)
	require.Equal(t, "1.2345678901", first.AmountUSD.String())

	replayed, err := repo.Append(ctx, firstInput)
	require.NoError(t, err)
	require.Equal(t, first.ID, replayed.ID)

	conflicting := transaction(key, "charge", "2")
	_, err = repo.Append(ctx, conflicting)
	require.ErrorIs(t, err, service.ErrBillingTransactionConflict)

	adjustment, err := repo.Append(ctx, transaction("transaction-adjustment-"+uuid.NewString(), "adjustment", "0.0000000001"))
	require.NoError(t, err)
	require.NotEqual(t, first.ID, adjustment.ID)

	byID, err := repo.GetByID(ctx, first.ID)
	require.NoError(t, err)
	byKey, err := repo.GetByKey(ctx, key)
	require.NoError(t, err)
	require.Equal(t, first.ID, byID.ID)
	require.Equal(t, first.ID, byKey.ID)

	items, err := repo.ListBySource(ctx, "video_task", 501, 10)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(items), 2)
	require.Equal(t, first.ID, items[0].ID)
	require.Equal(t, adjustment.ID, items[1].ID)
	require.Equal(t, "0.0000000001", items[1].AmountUSD.String())
}

func TestBillingTransactionRepositoryAppend_JSONNumericEquivalenceAndPrecision(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	repo := NewBillingTransactionRepository(integrationDB)
	user := mustCreateUser(t, client, &service.User{
		Email:        fmt.Sprintf("transaction-json-%s@example.com", uuid.NewString()),
		PasswordHash: "hash",
		Balance:      100,
	})
	exchangeRateAsOf := time.Now().UTC().Truncate(time.Microsecond)

	newTransaction := func(key string, metadata json.RawMessage) *service.BillingTransaction {
		return &service.BillingTransaction{
			TransactionKey:   key,
			SourceType:       "video_task",
			SourceID:         502,
			TransactionKind:  "charge",
			UserID:           user.ID,
			AmountOriginal:   service.MustUSD("1"),
			AmountUSD:        service.MustUSD("1"),
			ExchangeRate:     decimal.RequireFromString("1"),
			ExchangeRateAsOf: exchangeRateAsOf,
			PricingSource:    "provider_usage",
			PricingVersion:   "v1",
			BalanceBefore:    service.MustUSD("100"),
			BalanceAfter:     service.MustUSD("99"),
			Metadata:         metadata,
		}
	}

	equivalentKey := "transaction-json-equivalent-" + uuid.NewString()
	first, err := repo.Append(ctx, newTransaction(equivalentKey, json.RawMessage(`{"value":1,"nested":[1.0,{"n":1e0}]}`)))
	require.NoError(t, err)
	for _, metadata := range []json.RawMessage{
		json.RawMessage(`{"value":1.0,"nested":[1e0,{"n":1}]}`),
		json.RawMessage(`{"value":1e0,"nested":[1,{"n":1.00}]}`),
	} {
		replayed, err := repo.Append(ctx, newTransaction(equivalentKey, metadata))
		require.NoError(t, err)
		require.Equal(t, first.ID, replayed.ID)
	}

	largeKey := "transaction-json-large-" + uuid.NewString()
	_, err = repo.Append(ctx, newTransaction(largeKey, json.RawMessage(`{"value":9007199254740992}`)))
	require.NoError(t, err)
	_, err = repo.Append(ctx, newTransaction(largeKey, json.RawMessage(`{"value":9007199254740993}`)))
	require.ErrorIs(t, err, service.ErrBillingTransactionConflict)
}
