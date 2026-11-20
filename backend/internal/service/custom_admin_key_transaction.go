package service

import (
	"context"
	dbent "github.com/Wei-Shaw/sub2api/ent"
)

func lockScopedAdminAccount(ctx context.Context, tx *dbent.Tx, id int64) error {
	_, err := tx.Client().ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('scoped_admin_account:' || $1::text, 0))`, id)
	return err
}

// AfterAccountCommit 将不参与回滚的缓存和上游副作用延迟到事务提交成功后。
// 普通管理员没有外层事务时立即执行，保留原有行为。
func AfterAccountCommit(ctx context.Context, run func(context.Context)) {
	if tx := dbent.TxFromContext(ctx); tx != nil {
		tx.OnCommit(func(next dbent.Committer) dbent.Committer {
			return dbent.CommitFunc(func(commitCtx context.Context, tx *dbent.Tx) error {
				if err := next.Commit(commitCtx, tx); err != nil {
					return err
				}
				run(dbent.NewTxContext(ctx, nil))
				return nil
			})
		})
	} else {
		run(ctx)
	}
}
