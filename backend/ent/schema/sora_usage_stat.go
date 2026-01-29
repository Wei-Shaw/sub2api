// Package schema 定义 Ent ORM 的数据库 schema。
// 每个文件对应一个数据库实体（表），定义其字段、边（关联）和索引。
package schema

import (
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// SoraUsageStat 定义 Sora 调用统计表。
type SoraUsageStat struct {
	ent.Schema
REDACTED

// Annotations 返回 schema 的注解配置。
func (SoraUsageStat) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "sora_usage_stats"REDACTED,
REDACTED
REDACTED

// Mixin 返回该 schema 使用的混入组件。
func (SoraUsageStat) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{REDACTED,
REDACTED
REDACTED

// Fields 定义 SoraUsageStat 的字段。
func (SoraUsageStat) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("account_id").
			Comment("关联 accounts 表的 ID"),
		field.Int("image_count").
			Default(0),
		field.Int("video_count").
			Default(0),
		field.Int("error_count").
			Default(0),
		field.Time("last_error_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"REDACTED),
		field.Int("today_image_count").
			Default(0),
		field.Int("today_video_count").
			Default(0),
		field.Int("today_error_count").
			Default(0),
		field.Time("today_date").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "date"REDACTED),
		field.Int("consecutive_error_count").
			Default(0),
REDACTED
REDACTED

// Indexes 定义索引。
func (SoraUsageStat) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("account_id").Unique(),
		index.Fields("today_date"),
REDACTED
REDACTED
