package ent

import "context"

// WithoutTx 返回一个剥离了所附 *Tx 的 ctx 副本，使调用方可以在基础 client
// （autocommit）上执行 best-effort、非关键的副作用，而不加入——也就不会毒化——
// 外层事务。
//
// Postgres 语义：事务内任一语句失败即把整个事务标记为 aborted，后续语句全部被拒，
// 直到 ROLLBACK。因此对 fail-open 的副作用（如注册时的默认平台配额快照）必须做事务
// 隔离，否则一条无关紧要的写入失败会连累调用方的关键事务。
func WithoutTx(ctx context.Context) context.Context {
	if TxFromContext(ctx) == nil {
		return ctx
	}
	return context.WithValue(ctx, txCtxKey{}, (*Tx)(nil))
}
