//go:build integration

package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func newLedgerTestUser(t *testing.T, balance float64) int64 {
	t.Helper()
	client := testEntClient(t)
	user := mustCreateUser(t, client, &service.User{
		Email:        fmt.Sprintf("ledger-user-%d@example.com", time.Now().UnixNano()),
		PasswordHash: "hash",
		Balance:      balance,
	})
	return user.ID
}

func TestBalanceLedgerRepository_Deduct_NoOverdraft(t *testing.T) {
	ctx := context.Background()
	repo := NewBalanceLedgerRepository(integrationDB)
	userID := newLedgerTestUser(t, 50)

	// 余额不足直接拒绝，余额不变（不透支）。
	_, err := repo.Deduct(ctx, &service.LedgerDeductCommand{
		AppID: "app-a", RequestID: "d-over", UserID: userID, Amount: 100, Description: "too much",
	})
	require.ErrorIs(t, err, service.ErrBalanceInsufficient)

	var balance float64
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT balance FROM users WHERE id=$1`, userID).Scan(&balance))
	require.InDelta(t, 50, balance, 1e-9)
}

func TestBalanceLedgerRepository_Deduct_Idempotent(t *testing.T) {
	ctx := context.Background()
	repo := NewBalanceLedgerRepository(integrationDB)
	userID := newLedgerTestUser(t, 100)

	first, err := repo.Deduct(ctx, &service.LedgerDeductCommand{
		AppID: "app-a", RequestID: "d1", UserID: userID, Amount: 30, Description: "buy",
	})
	require.NoError(t, err)
	require.True(t, first.Applied)
	require.InDelta(t, 70, first.BalanceAfter, 1e-9)

	// 相同 request_id 幂等重放：不重复扣，返回首次余额。
	replay, err := repo.Deduct(ctx, &service.LedgerDeductCommand{
		AppID: "app-a", RequestID: "d1", UserID: userID, Amount: 30, Description: "buy",
	})
	require.NoError(t, err)
	require.False(t, replay.Applied)
	require.InDelta(t, 70, replay.BalanceAfter, 1e-9)

	var balance float64
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT balance FROM users WHERE id=$1`, userID).Scan(&balance))
	require.InDelta(t, 70, balance, 1e-9)
}

func TestBalanceLedgerRepository_Refund_PartialAndOverRefund(t *testing.T) {
	ctx := context.Background()
	repo := NewBalanceLedgerRepository(integrationDB)
	userID := newLedgerTestUser(t, 100)

	_, err := repo.Deduct(ctx, &service.LedgerDeductCommand{
		AppID: "app-a", RequestID: "d1", UserID: userID, Amount: 30, Description: "buy",
	})
	require.NoError(t, err)

	// 部分退 10。
	r1, err := repo.Refund(ctx, &service.LedgerRefundCommand{
		AppID: "app-a", RefundRequestID: "r1", OriginalRequestID: "d1", Amount: 10, Description: "partial",
	})
	require.NoError(t, err)
	require.InDelta(t, 10, r1.RefundedTotal, 1e-9)
	require.InDelta(t, 80, r1.BalanceAfter, 1e-9)
	require.Equal(t, userID, r1.UserID)

	// 累计超额（10+25 > 30）被拒。
	_, err = repo.Refund(ctx, &service.LedgerRefundCommand{
		AppID: "app-a", RefundRequestID: "r2", OriginalRequestID: "d1", Amount: 25, Description: "over",
	})
	require.ErrorIs(t, err, service.ErrOverRefund)

	// 退到刚好全额（10+20=30）允许。
	r3, err := repo.Refund(ctx, &service.LedgerRefundCommand{
		AppID: "app-a", RefundRequestID: "r3", OriginalRequestID: "d1", Amount: 20, Description: "rest",
	})
	require.NoError(t, err)
	require.InDelta(t, 30, r3.RefundedTotal, 1e-9)
	require.InDelta(t, 100, r3.BalanceAfter, 1e-9)

	// 退款幂等重放（相同 refund_request_id 不重复退）。
	replay, err := repo.Refund(ctx, &service.LedgerRefundCommand{
		AppID: "app-a", RefundRequestID: "r1", OriginalRequestID: "d1", Amount: 10, Description: "partial",
	})
	require.NoError(t, err)
	require.False(t, replay.Applied)

	var balance float64
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT balance FROM users WHERE id=$1`, userID).Scan(&balance))
	require.InDelta(t, 100, balance, 1e-9)
}

func TestBalanceLedgerRepository_Refund_OriginalNotFoundOrCrossApp(t *testing.T) {
	ctx := context.Background()
	repo := NewBalanceLedgerRepository(integrationDB)
	userID := newLedgerTestUser(t, 100)

	_, err := repo.Deduct(ctx, &service.LedgerDeductCommand{
		AppID: "app-a", RequestID: "d1", UserID: userID, Amount: 30, Description: "buy",
	})
	require.NoError(t, err)

	// 原扣不存在。
	_, err = repo.Refund(ctx, &service.LedgerRefundCommand{
		AppID: "app-a", RefundRequestID: "r1", OriginalRequestID: "nope", Amount: 1, Description: "x",
	})
	require.ErrorIs(t, err, service.ErrOriginalDeductNotFound)

	// 不能冲销其它 app 的扣费。
	_, err = repo.Refund(ctx, &service.LedgerRefundCommand{
		AppID: "app-b", RefundRequestID: "r2", OriginalRequestID: "d1", Amount: 1, Description: "x",
	})
	require.ErrorIs(t, err, service.ErrOriginalDeductNotFound)
}
