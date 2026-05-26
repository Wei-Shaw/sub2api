package pluginsdk

import (
	"context"
	"database/sql"
	"fmt"
)

// WithTx executes fn inside a transaction. If fn returns nil the
// transaction is committed; if fn returns an error or panics the
// transaction is rolled back.
//
// The context is wrapped with context.WithoutCancel to prevent HTTP
// handler context cancellation from triggering an automatic rollback by
// the Go runtime (a constraint of the gRPC SQL proxy's context
// lifecycle). Plugin authors should prefer WithTx over hand-written
// db.BeginTx + defer rollback + commit.
func WithTx(ctx context.Context, db *sql.DB, fn func(tx *sql.Tx) error) error {
	return WithTxOpts(ctx, db, nil, fn)
}

// WithTxOpts is like WithTx but accepts transaction options (isolation
// level, read-only, etc.).
func WithTxOpts(ctx context.Context, db *sql.DB, opts *sql.TxOptions, fn func(tx *sql.Tx) error) (err error) {
	tx, err := db.BeginTx(context.WithoutCancel(ctx), opts)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		if p := recover(); p != nil {
			if rbErr := tx.Rollback(); rbErr != nil {
				// Log-worthy but the panic takes priority.
				_ = rbErr
			}
			panic(p)
		}
		if err != nil {
			if rbErr := tx.Rollback(); rbErr != nil {
				err = fmt.Errorf("%w (rollback also failed: %v)", err, rbErr)
			}
			return
		}
		err = tx.Commit()
	}()
	err = fn(tx)
	return
}
