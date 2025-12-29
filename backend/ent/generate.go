package ent

// 启用 sql/execquery 以生成 ExecContext/QueryContext 的透传接口，便于事务内执行原生 SQL。
//go:generate go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert,intercept,sql/execquery --idtype int64 ./schema
