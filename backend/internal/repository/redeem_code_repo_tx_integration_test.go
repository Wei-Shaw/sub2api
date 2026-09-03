//go:build integration

package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

// TestRedeemCodeRepositoryCreateJoinsCallerTransaction 钉住 M8：
// 充值履约要把「创建充值码」和「兑换」放进同一个事务，前提是 Create/GetByCode/GetByID
// 复用 context 里的事务。Create 原先直接用 r.client，创建永远落在事务之外——
// 事务回滚后仍会留下一条 unused 的孤儿码，而随后的 Redeem 在事务里根本看不到它。
func TestRedeemCodeRepositoryCreateJoinsCallerTransaction(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	repo := NewRedeemCodeRepository(client)

	code := fmt.Sprintf("PAY-TXATOMIC-%d", time.Now().UnixNano())

	tx, err := client.Tx(ctx)
	require.NoError(t, err)
	txCtx := dbent.NewTxContext(ctx, tx)

	require.NoError(t, repo.Create(txCtx, &service.RedeemCode{
		Code:   code,
		Type:   service.RedeemTypeBalance,
		Value:  10,
		Status: service.StatusUnused,
	}))

	// 事务内可见：Redeem 复用同一事务时才能查到刚创建的码。
	got, err := repo.GetByCode(txCtx, code)
	require.NoError(t, err)
	require.Equal(t, code, got.Code)
	byID, err := repo.GetByID(txCtx, got.ID)
	require.NoError(t, err)
	require.Equal(t, code, byID.Code)

	// 事务外不可见：还没提交。
	_, err = repo.GetByCode(ctx, code)
	require.ErrorIs(t, err, service.ErrRedeemCodeNotFound)

	require.NoError(t, tx.Rollback())

	// 回滚之后彻底消失，不留下可被兑换却没有对应入账的孤儿码。
	_, err = repo.GetByCode(ctx, code)
	require.ErrorIs(t, err, service.ErrRedeemCodeNotFound)
}

// TestRedeemCodeRepositoryUseIsAtomicWithCreateInSameTransaction 创建 + 标记已用
// 必须一起提交或一起回滚。
func TestRedeemCodeRepositoryUseIsAtomicWithCreateInSameTransaction(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	repo := NewRedeemCodeRepository(client)

	user := mustCreateUser(t, client, &service.User{
		Email: fmt.Sprintf("redeem-tx-%d@example.com", time.Now().UnixNano()),
	})

	code := fmt.Sprintf("PAY-TXCOMMIT-%d", time.Now().UnixNano())

	tx, err := client.Tx(ctx)
	require.NoError(t, err)
	txCtx := dbent.NewTxContext(ctx, tx)

	created := &service.RedeemCode{
		Code:   code,
		Type:   service.RedeemTypeBalance,
		Value:  10,
		Status: service.StatusUnused,
	}
	require.NoError(t, repo.Create(txCtx, created))
	require.NoError(t, repo.Use(txCtx, created.ID, user.ID))
	require.NoError(t, tx.Commit())

	persisted, err := repo.GetByCode(ctx, code)
	require.NoError(t, err)
	require.Equal(t, service.StatusUsed, persisted.Status)
	require.NotNil(t, persisted.UsedBy)
	require.Equal(t, user.ID, *persisted.UsedBy)

	// 条件 UPDATE ... WHERE status='unused' 仍是唯一的双花防线。
	require.ErrorIs(t, repo.Use(ctx, created.ID, user.ID), service.ErrRedeemCodeUsed)
}
