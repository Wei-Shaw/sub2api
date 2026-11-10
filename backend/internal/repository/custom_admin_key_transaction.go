package repository

import (
	"context"
	dbent "github.com/Wei-Shaw/sub2api/ent"
)

// accountTransactionScope 同时切换 ORM、原生 SQL 和关联查询，禁止在提交前发布缓存。
func (r *accountRepository) accountTransactionScope(ctx context.Context) *accountRepository {
	tx := dbent.TxFromContext(ctx)
	if tx == nil || r.client == tx.Client() {
		return r
	}
	scoped := *r
	scoped.client = tx.Client()
	scoped.sql = tx.Client()
	scoped.schedulerCache = nil
	return &scoped
}
