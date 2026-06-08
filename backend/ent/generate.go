// Package ent provides the generated ORM code for database entities.
package ent

// 启用 sql/execquery 以生成 ExecContext/QueryContext 的透传接口，便于事务内执行原生 SQL。
// 启用 sql/lock 以支持 FOR UPDATE 行锁。
//go:generate go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert,intercept,sql/execquery,sql/lock --idtype int64 ./schema
