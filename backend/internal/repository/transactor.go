package repository

import (
	"context"
	"fmt"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// entTransactionRunner wraps an *ent.Client to implement service.TransactionRunner.
// It propagates the Ent Tx via context so that repositories using txAwareSQLExecutor
// automatically participate in the same transaction.
type entTransactionRunner struct {
	client *dbent.Client
}

// NewEntTransactionRunner constructs an EntTransactionRunner and returns it as
// service.TransactionRunner.  Wire providers should call this directly.
func NewEntTransactionRunner(client *dbent.Client) service.TransactionRunner {
	return &entTransactionRunner{client: client}
}

func (r *entTransactionRunner) RunInTx(ctx context.Context, fn func(ctx context.Context) error) error {
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return fmt.Errorf("begin ent tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	txCtx := dbent.NewTxContext(ctx, tx)
	if err := fn(txCtx); err != nil {
		return err
	}
	return tx.Commit()
}
