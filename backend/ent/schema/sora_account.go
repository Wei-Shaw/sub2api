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

// SoraAccount 定义 Sora 账号扩展表。
type SoraAccount struct {
	ent.Schema
REDACTED

// Annotations 返回 schema 的注解配置。
func (SoraAccount) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "sora_accounts"REDACTED,
REDACTED
REDACTED

// Mixin 返回该 schema 使用的混入组件。
func (SoraAccount) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{REDACTED,
REDACTED
REDACTED

// Fields 定义 SoraAccount 的字段。
func (SoraAccount) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("account_id").
			Comment("关联 accounts 表的 ID"),
		field.String("access_token").
			Optional().
			Nillable(),
		field.String("session_token").
			Optional().
			Nillable(),
		field.String("refresh_token").
			Optional().
			Nillable(),
		field.String("client_id").
			Optional().
			Nillable(),
		field.String("email").
			Optional().
			Nillable(),
		field.String("username").
			Optional().
			Nillable(),
		field.String("remark").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "text"REDACTED),
		field.Int("use_count").
			Default(0),
		field.String("plan_type").
			Optional().
			Nillable(),
		field.String("plan_title").
			Optional().
			Nillable(),
		field.Time("subscription_end").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"REDACTED),
		field.Bool("sora_supported").
			Default(false),
		field.String("sora_invite_code").
			Optional().
			Nillable(),
		field.Int("sora_redeemed_count").
			Default(0),
		field.Int("sora_remaining_count").
			Default(0),
		field.Int("sora_total_count").
			Default(0),
		field.Time("sora_cooldown_until").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"REDACTED),
		field.Time("cooled_until").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"REDACTED),
		field.Bool("image_enabled").
			Default(true),
		field.Bool("video_enabled").
			Default(true),
		field.Int("image_concurrency").
			Default(-1),
		field.Int("video_concurrency").
			Default(-1),
		field.Bool("is_expired").
			Default(false),
REDACTED
REDACTED

// Indexes 定义索引。
func (SoraAccount) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("account_id").Unique(),
		index.Fields("plan_type"),
		index.Fields("sora_supported"),
		index.Fields("image_enabled"),
		index.Fields("video_enabled"),
REDACTED
REDACTED
