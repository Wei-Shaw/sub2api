package pluginsdk

import (
	"context"
	"database/sql"
	"fmt"
)

// WithTx 在一个事务内执行 fn。如果 fn 返回 nil，事务 commit；
// 如果 fn 返回 error 或 panic，事务 rollback。
//
// ctx 会被 context.WithoutCancel 包装以防止 HTTP handler context
// 取消导致事务被 Go runtime 自动 rollback（gRPC SQL 代理的 context
// 生命周期限制）。Plugin 作者应优先使用 WithTx 而非手写
// db.BeginTx + defer rollback + commit。
func WithTx(ctx context.Context, db *sql.DB, fn func(tx *sql.Tx) error) (err error) {
	tx, err := db.BeginTx(context.WithoutCancel(ctx), nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p) // re-throw after rollback
		}
		if err != nil {
			_ = tx.Rollback()
			return
		}
		err = tx.Commit()
	}()
	err = fn(tx)
	return
}

// WithTxOpts 同 WithTx 但允许指定事务选项（隔离级别、只读等）。
func WithTxOpts(ctx context.Context, db *sql.DB, opts *sql.TxOptions, fn func(tx *sql.Tx) error) (err error) {
	tx, err := db.BeginTx(context.WithoutCancel(ctx), opts)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p)
		}
		if err != nil {
			_ = tx.Rollback()
			return
		}
		err = tx.Commit()
	}()
	err = fn(tx)
	return
}
