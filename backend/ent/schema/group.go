package schema

import (
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"
	"github.com/Wei-Shaw/sub2api/internal/domain"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Group holds the schema definition for the Group entity.
type Group struct {
	ent.Schema
REDACTED

func (Group) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "groups"REDACTED,
REDACTED
REDACTED

func (Group) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{REDACTED,
		mixins.SoftDeleteMixin{REDACTED,
REDACTED
REDACTED

func (Group) Fields() []ent.Field {
	return []ent.Field{
		// 唯一约束通过部分索引实现（WHERE deleted_at IS NULL），支持软删除后重用
		// 见迁移文件 016_soft_delete_partial_unique_indexes.sql
		field.String("name").
			MaxLen(100).
			NotEmpty(),
		field.String("description").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "text"REDACTED),
		field.Float("rate_multiplier").
			SchemaType(map[string]string{dialect.Postgres: "decimal(10,4)"REDACTED).
			Default(1.0),
		field.Bool("is_exclusive").
			Default(false),
		field.String("status").
			MaxLen(20).
			Default(domain.StatusActive),

		// Subscription-related fields (added by migration 003)
		field.String("platform").
			MaxLen(50).
			Default(domain.PlatformAnthropic),
		field.String("subscription_type").
			MaxLen(20).
			Default(domain.SubscriptionTypeStandard),
		field.Float("daily_limit_usd").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"REDACTED),
		field.Float("weekly_limit_usd").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"REDACTED),
		field.Float("monthly_limit_usd").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"REDACTED),
		field.Int("default_validity_days").
			Default(30),

		// 图片生成计费配置（antigravity 和 gemini 平台使用）
		field.Float("image_price_1k").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"REDACTED),
		field.Float("image_price_2k").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"REDACTED),
		field.Float("image_price_4k").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"REDACTED),

		// Sora 按次计费配置（阶段 1）
		field.Float("sora_image_price_360").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"REDACTED),
		field.Float("sora_image_price_540").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"REDACTED),
		field.Float("sora_video_price_per_request").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"REDACTED),
		field.Float("sora_video_price_per_request_hd").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"REDACTED),

		// Claude Code 客户端限制 (added by migration 029)
		field.Bool("claude_code_only").
			Default(false).
			Comment("是否仅允许 Claude Code 客户端"),
		field.Int64("fallback_group_id").
			Optional().
			Nillable().
			Comment("非 Claude Code 请求降级使用的分组 ID"),

		// 模型路由配置 (added by migration 040)
		field.JSON("model_routing", map[string][]int64{REDACTED).
			Optional().
			SchemaType(map[string]string{dialect.Postgres: "jsonb"REDACTED).
			Comment("模型路由配置：模型模式 -> 优先账号ID列表"),

		// 模型路由开关 (added by migration 041)
		field.Bool("model_routing_enabled").
			Default(false).
			Comment("是否启用模型路由配置"),
REDACTED
REDACTED

func (Group) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("api_keys", APIKey.Type),
		edge.To("redeem_codes", RedeemCode.Type),
		edge.To("subscriptions", UserSubscription.Type),
		edge.To("usage_logs", UsageLog.Type),
		edge.From("accounts", Account.Type).
			Ref("groups").
			Through("account_groups", AccountGroup.Type),
		edge.From("allowed_users", User.Type).
			Ref("allowed_groups").
			Through("user_allowed_groups", UserAllowedGroup.Type),
		// 注意：fallback_group_id 直接作为字段使用，不定义 edge
		// 这样允许多个分组指向同一个降级分组（M2O 关系）
REDACTED
REDACTED

func (Group) Indexes() []ent.Index {
	return []ent.Index{
		// name 字段已在 Fields() 中声明 Unique()，无需重复索引
		index.Fields("status"),
		index.Fields("platform"),
		index.Fields("subscription_type"),
		index.Fields("is_exclusive"),
		index.Fields("deleted_at"),
REDACTED
REDACTED
