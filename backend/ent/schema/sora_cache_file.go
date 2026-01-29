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

// SoraCacheFile 定义 Sora 缓存文件表。
type SoraCacheFile struct {
	ent.Schema
REDACTED

// Annotations 返回 schema 的注解配置。
func (SoraCacheFile) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "sora_cache_files"REDACTED,
REDACTED
REDACTED

// Fields 定义 SoraCacheFile 的字段。
func (SoraCacheFile) Fields() []ent.Field {
	return []ent.Field{
		field.String("task_id").
			MaxLen(120).
			Optional().
			Nillable(),
		field.Int64("account_id"),
		field.Int64("user_id"),
		field.String("media_type").
			MaxLen(32),
		field.String("original_url").
			SchemaType(map[string]string{dialect.Postgres: "text"REDACTED),
		field.String("cache_path").
			SchemaType(map[string]string{dialect.Postgres: "text"REDACTED),
		field.String("cache_url").
			SchemaType(map[string]string{dialect.Postgres: "text"REDACTED),
		field.Int64("size_bytes").
			Default(0),
		field.Time("created_at").
			Default(time.Now).
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"REDACTED),
REDACTED
REDACTED

// Indexes 定义索引。
func (SoraCacheFile) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("account_id"),
		index.Fields("user_id"),
		index.Fields("media_type"),
REDACTED
REDACTED
