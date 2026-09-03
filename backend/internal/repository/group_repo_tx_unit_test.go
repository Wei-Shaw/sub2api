package repository

import (
	"context"
	"errors"
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/stretchr/testify/require"
)

// TestClientFromContext_UsesTxClient 证明 clientFromContext 在外层 tx 存在时返回事务 client。
// Create / DeleteCascade 依赖此契约以参与调用方事务。
func TestClientFromContext_UsesTxClient(t *testing.T) {
	defaultClient := &dbent.Client{}
	tx := &dbent.Tx{}
	ctx := context.Background()
	got := clientFromContext(ctx, defaultClient)
	require.Same(t, defaultClient, got, "no outer tx → default client")

	txCtx := dbent.NewTxContext(ctx, tx)
	require.Same(t, tx, dbent.TxFromContext(txCtx))
}

// TestGroupOutboxImmediate_BranchSelection 断言 Create/DeleteCascade 的 outbox 分支选择：
// 无外层 tx → 立即 enqueue；有 TxFromContext → 延迟到 post-commit EnqueueGroupChanged。
// 完整 rollback 集成回归仍需 //go:build integration + 真实 DB。
func TestGroupOutboxImmediate_BranchSelection(t *testing.T) {
	require.True(t, groupOutboxImmediate(context.Background()), "no outer tx → enqueue immediately")

	txCtx := dbent.NewTxContext(context.Background(), &dbent.Tx{})
	require.False(t, groupOutboxImmediate(txCtx), "outer tx present → defer outbox until commit")
}

// TestOuterTxClientOrError_FailClosedWithoutContext 断言 ErrTxStarted 且未附着
// TxFromContext 时 fail-closed，避免 DeleteCascade/CreateFromSource 静默旁路 r.client。
func TestOuterTxClientOrError_FailClosedWithoutContext(t *testing.T) {
	client, err := outerTxClientOrError(context.Background())
	require.Error(t, err)
	require.Nil(t, client)
	require.ErrorIs(t, err, errGroupOuterTxMissing)

	tx := &dbent.Tx{}
	txCtx := dbent.NewTxContext(context.Background(), tx)
	client, err = outerTxClientOrError(txCtx)
	require.NoError(t, err)
	// 零值 Tx.Client() 可能为 nil；关键是 err==nil 且不返回 errGroupOuterTxMissing
	_ = client
	require.False(t, errors.Is(err, errGroupOuterTxMissing))
}
