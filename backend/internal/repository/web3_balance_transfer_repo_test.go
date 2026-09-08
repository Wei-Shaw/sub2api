package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/web3deposit"
	"github.com/stretchr/testify/require"
)

func TestWeb3BalanceTransferRepositoryCreateAndGet(t *testing.T) {
	client := newWeb3BalanceTestClient(t)
	balanceRepo := NewWeb3UserBalanceRepository(client)
	transferRepo := NewWeb3BalanceTransferRepository(client)
	ctx := context.Background()

	balance, err := balanceRepo.CreateOrGet(ctx, 42, web3deposit.AssetKeyUSDT)
	require.NoError(t, err)

	createdAt := time.Date(2026, time.August, 7, 12, 0, 0, 0, time.UTC)
	created, err := transferRepo.Create(ctx, testWeb3BalanceTransfer(balance.ID, "web3-transfer:42:1", createdAt))
	require.NoError(t, err)
	require.Positive(t, created.ID)
	require.Equal(t, "4.00000000", created.Amount)
	require.Equal(t, map[string]any{"source": "manual"}, created.Metadata)
	require.Equal(t, createdAt, created.CreatedAt)

	loaded, err := transferRepo.GetByIdempotencyKey(ctx, created.IdempotencyKey)
	require.NoError(t, err)
	require.Equal(t, created.ID, loaded.ID)
	require.Equal(t, created.Web3BalanceAfter, loaded.Web3BalanceAfter)
}

func TestWeb3BalanceTransferRepositoryEnforcesIdempotency(t *testing.T) {
	client := newWeb3BalanceTestClient(t)
	balanceRepo := NewWeb3UserBalanceRepository(client)
	transferRepo := NewWeb3BalanceTransferRepository(client)
	ctx := context.Background()

	balance, err := balanceRepo.CreateOrGet(ctx, 42, web3deposit.AssetKeyUSDT)
	require.NoError(t, err)
	transfer := testWeb3BalanceTransfer(balance.ID, "web3-transfer:42:1", time.Time{})

	_, err = transferRepo.Create(ctx, transfer)
	require.NoError(t, err)
	_, err = transferRepo.Create(ctx, transfer)
	require.ErrorIs(t, err, web3deposit.ErrTransferAlreadyExists)
}

func TestWeb3BalanceTransferRepositoryListsNewestFirst(t *testing.T) {
	client := newWeb3BalanceTestClient(t)
	balanceRepo := NewWeb3UserBalanceRepository(client)
	transferRepo := NewWeb3BalanceTransferRepository(client)
	ctx := context.Background()

	balance, err := balanceRepo.CreateOrGet(ctx, 42, web3deposit.AssetKeyUSDT)
	require.NoError(t, err)
	older := time.Date(2026, time.August, 7, 10, 0, 0, 0, time.UTC)
	newer := older.Add(time.Hour)
	_, err = transferRepo.Create(ctx, testWeb3BalanceTransfer(balance.ID, "web3-transfer:42:1", older))
	require.NoError(t, err)
	_, err = transferRepo.Create(ctx, testWeb3BalanceTransfer(balance.ID, "web3-transfer:42:2", newer))
	require.NoError(t, err)

	transfers, err := transferRepo.ListByUser(ctx, 42)
	require.NoError(t, err)
	require.Len(t, transfers, 2)
	require.Equal(t, "web3-transfer:42:2", transfers[0].IdempotencyKey)
	require.Equal(t, "web3-transfer:42:1", transfers[1].IdempotencyKey)
}

func TestWeb3BalanceTransferRepositoryReturnsNotFound(t *testing.T) {
	repo := NewWeb3BalanceTransferRepository(newWeb3BalanceTestClient(t))

	_, err := repo.GetByIdempotencyKey(context.Background(), "missing")
	require.True(t, errors.Is(err, web3deposit.ErrTransferNotFound))
}

func testWeb3BalanceTransfer(balanceID int64, idempotencyKey string, createdAt time.Time) web3deposit.BalanceTransfer {
	return web3deposit.BalanceTransfer{
		UserID:            42,
		Web3BalanceID:     balanceID,
		Amount:            "4.00000000",
		Web3BalanceBefore: "10.00000000",
		Web3BalanceAfter:  "6.00000000",
		UserBalanceBefore: "3.00000000",
		UserBalanceAfter:  "7.00000000",
		IdempotencyKey:    idempotencyKey,
		Metadata:          map[string]any{"source": "manual"},
		CreatedAt:         createdAt,
	}
}
