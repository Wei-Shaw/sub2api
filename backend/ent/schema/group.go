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
}

func (Group) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "groups"},
	}
}

func (Group) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
		mixins.SoftDeleteMixin{},
	}
}

func (Group) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").
			MaxLen(100).
			NotEmpty(),
		field.String("description").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "text"}),
		field.Float("rate_multiplier").
			SchemaType(map[string]string{dialect.Postgres: "decimal(10,4)"}).
			Default(1.0),
		field.Bool("is_exclusive").
			Default(false),
		field.String("status").
			MaxLen(20).
			Default(domain.StatusActive),

		field.String("platform").
			MaxLen(50).
			Default(domain.PlatformAnthropic),
		field.String("subscription_type").
			MaxLen(20).
			Default(domain.SubscriptionTypeStandard),
		field.Float("daily_limit_usd").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}),
		field.Float("weekly_limit_usd").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}),
		field.Float("monthly_limit_usd").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}),
		field.Int("default_validity_days").
			Default(30),

		field.Float("image_price_1k").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}),
		field.Float("image_price_2k").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}),
		field.Float("image_price_4k").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}),

		field.Float("sora_image_price_360").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}),
		field.Float("sora_image_price_540").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}),
		field.Float("sora_video_price_per_request").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}),
		field.Float("sora_video_price_per_request_hd").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}),

		// Sora 存储配额
		field.Int64("sora_storage_quota_bytes").
			Default(0),

		field.Bool("claude_code_only").
			Default(false).
			Comment("allow Claude Code client only"),
		field.Int64("fallback_group_id").
			Optional().
			Nillable().
			Comment("fallback group for non-Claude-Code requests"),
		field.Int64("fallback_group_id_on_invalid_request").
			Optional().
			Nillable().
			Comment("fallback group for invalid request"),

		field.JSON("model_routing", map[string][]int64{}).
			Optional().
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}).
			Comment("model routing config: pattern -> account ids"),
		field.Bool("model_routing_enabled").
			Default(false).
			Comment("whether model routing is enabled"),

		field.Bool("mcp_xml_inject").
			Default(true).
			Comment("whether MCP XML prompt injection is enabled"),

		field.JSON("supported_model_scopes", []string{}).
			Default([]string{"claude", "gemini_text", "gemini_image"}).
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}).
			Comment("supported model scopes: claude, gemini_text, gemini_image"),

		field.Int("sort_order").
			Default(0).
			Comment("group display order, lower comes first"),

		// OpenAI Messages 调度配置 (added by migration 069)
		field.Bool("allow_messages_dispatch").
			Default(false).
			Comment("是否允许 /v1/messages 调度到此 OpenAI 分组"),
		field.String("default_mapped_model").
			MaxLen(100).
			Default("").
			Comment("默认映射模型 ID，当账号级映射找不到时使用此值"),
		field.Bool("simulate_claude_max_enabled").
			Default(false).
			Comment("simulate claude usage as claude-max style (1h cache write)"),
	}
}

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
	}
}

func (Group) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("status"),
		index.Fields("platform"),
		index.Fields("subscription_type"),
		index.Fields("is_exclusive"),
		index.Fields("deleted_at"),
		index.Fields("sort_order"),
	}
}
