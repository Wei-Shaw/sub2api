//go:build integration

package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func TestUserRepositoryTransferBalance_AppliesAtomicTransfer(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	repo := newUserRepositoryWithSQL(client, integrationDB)

	from := mustCreateUser(t, client, &service.User{
		Email:        fmt.Sprintf("transfer-from-%d@example.com", time.Now().UnixNano()),
		PasswordHash: "hash",
		Balance:      10,
	})
	to := mustCreateUser(t, client, &service.User{
		Email:        fmt.Sprintf("transfer-to-%d@example.com", time.Now().UnixNano()),
		PasswordHash: "hash",
		Balance:      2,
	})

	transfer, err := repo.TransferBalance(ctx, service.BalanceTransferInput{
		ExternalID: "forum-read-" + uuid.NewString(),
		FromUserID: from.ID,
		ToUserID:   to.ID,
		Amount:     0.000123,
		Reason:     "forum_read",
		Metadata:   map[string]any{"post_id": "post_1"},
	})
	require.NoError(t, err)
	require.NotZero(t, transfer.ID)
	require.InDelta(t, 9.999877, transfer.FromBalance, 0.000000001)
	require.InDelta(t, 2.000123, transfer.ToBalance, 0.000000001)

	var fromBalance, toBalance float64
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT balance FROM users WHERE id = $1", from.ID).Scan(&fromBalance))
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT balance FROM users WHERE id = $1", to.ID).Scan(&toBalance))
	require.InDelta(t, 9.999877, fromBalance, 0.000000001)
	require.InDelta(t, 2.000123, toBalance, 0.000000001)
}

func TestUserRepositoryTransferBalance_ReplaysExternalID(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	repo := newUserRepositoryWithSQL(client, integrationDB)

	from := mustCreateUser(t, client, &service.User{
		Email:        fmt.Sprintf("transfer-replay-from-%d@example.com", time.Now().UnixNano()),
		PasswordHash: "hash",
		Balance:      10,
	})
	to := mustCreateUser(t, client, &service.User{
		Email:        fmt.Sprintf("transfer-replay-to-%d@example.com", time.Now().UnixNano()),
		PasswordHash: "hash",
		Balance:      2,
	})
	input := service.BalanceTransferInput{
		ExternalID: "forum-read-" + uuid.NewString(),
		FromUserID: from.ID,
		ToUserID:   to.ID,
		Amount:     1.25,
		Reason:     "forum_read",
	}

	first, err := repo.TransferBalance(ctx, input)
	require.NoError(t, err)
	second, err := repo.TransferBalance(ctx, input)
	require.NoError(t, err)
	require.Equal(t, first.ID, second.ID)
	require.InDelta(t, first.FromBalance, second.FromBalance, 0.000000001)
	require.InDelta(t, first.ToBalance, second.ToBalance, 0.000000001)

	var fromBalance, toBalance float64
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT balance FROM users WHERE id = $1", from.ID).Scan(&fromBalance))
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT balance FROM users WHERE id = $1", to.ID).Scan(&toBalance))
	require.InDelta(t, 8.75, fromBalance, 0.000000001)
	require.InDelta(t, 3.25, toBalance, 0.000000001)
}

func TestUserRepositoryTransferBalance_RejectsExternalIDConflict(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	repo := newUserRepositoryWithSQL(client, integrationDB)

	from := mustCreateUser(t, client, &service.User{
		Email:        fmt.Sprintf("transfer-conflict-from-%d@example.com", time.Now().UnixNano()),
		PasswordHash: "hash",
		Balance:      10,
	})
	to := mustCreateUser(t, client, &service.User{
		Email:        fmt.Sprintf("transfer-conflict-to-%d@example.com", time.Now().UnixNano()),
		PasswordHash: "hash",
		Balance:      2,
	})
	input := service.BalanceTransferInput{
		ExternalID: "forum-read-" + uuid.NewString(),
		FromUserID: from.ID,
		ToUserID:   to.ID,
		Amount:     1.25,
		Reason:     "forum_read",
	}

	_, err := repo.TransferBalance(ctx, input)
	require.NoError(t, err)

	input.Amount = 1.5
	_, err = repo.TransferBalance(ctx, input)
	require.ErrorIs(t, err, service.ErrBalanceTransferConflict)

	var fromBalance, toBalance float64
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT balance FROM users WHERE id = $1", from.ID).Scan(&fromBalance))
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT balance FROM users WHERE id = $1", to.ID).Scan(&toBalance))
	require.InDelta(t, 8.75, fromBalance, 0.000000001)
	require.InDelta(t, 3.25, toBalance, 0.000000001)
}

func TestUserRepositoryTransferBalance_InsufficientBalanceDoesNotCreditRecipient(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	repo := newUserRepositoryWithSQL(client, integrationDB)

	from := mustCreateUser(t, client, &service.User{
		Email:        fmt.Sprintf("transfer-low-from-%d@example.com", time.Now().UnixNano()),
		PasswordHash: "hash",
		Balance:      1,
	})
	to := mustCreateUser(t, client, &service.User{
		Email:        fmt.Sprintf("transfer-low-to-%d@example.com", time.Now().UnixNano()),
		PasswordHash: "hash",
		Balance:      2,
	})

	_, err := repo.TransferBalance(ctx, service.BalanceTransferInput{
		ExternalID: "forum-read-" + uuid.NewString(),
		FromUserID: from.ID,
		ToUserID:   to.ID,
		Amount:     1.25,
		Reason:     "forum_read",
	})
	require.ErrorIs(t, err, service.ErrInsufficientBalance)

	var fromBalance, toBalance float64
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT balance FROM users WHERE id = $1", from.ID).Scan(&fromBalance))
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT balance FROM users WHERE id = $1", to.ID).Scan(&toBalance))
	require.InDelta(t, 1, fromBalance, 0.000000001)
	require.InDelta(t, 2, toBalance, 0.000000001)
}
