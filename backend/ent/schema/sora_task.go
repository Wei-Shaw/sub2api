// Package schema 定义 Ent ORM 的数据库 schema。
// 每个文件对应一个数据库实体（表），定义其字段、边（关联）和索引。
package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// SoraTask 定义 Sora 任务记录表。
type SoraTask struct {
	ent.Schema
REDACTED

// Annotations 返回 schema 的注解配置。
func (SoraTask) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "sora_tasks"REDACTED,
REDACTED
REDACTED

// Fields 定义 SoraTask 的字段。
func (SoraTask) Fields() []ent.Field {
	return []ent.Field{
		field.String("task_id").
			MaxLen(120).
			Unique(),
		field.Int64("account_id"),
		field.String("model").
			MaxLen(120),
		field.String("prompt").
			SchemaType(map[string]string{dialect.Postgres: "text"REDACTED),
		field.String("status").
			MaxLen(32).
			Default("processing"),
		field.Float("progress").
			Default(0),
		field.String("result_urls").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "text"REDACTED),
		field.String("error_message").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "text"REDACTED),
		field.Int("retry_count").
			Default(0),
		field.Time("created_at").
			Default(time.Now).
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"REDACTED),
		field.Time("completed_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"REDACTED),
REDACTED
REDACTED

// Indexes 定义索引。
func (SoraTask) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("account_id"),
		index.Fields("status"),
REDACTED
REDACTED
