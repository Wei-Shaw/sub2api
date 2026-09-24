package schema

import (
	"fmt"

	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// APIKeyPlatformUsage 记录单个 API Key 在单个上游来源（平台）上的用量与窗口。
//
// 限额本身存在 api_keys.platform_limits（JSONB），这里只存用量，便于热路径按
// (api_key_id, platform) 单行读写，且只对显式配置了平台限额的组合建行。
type APIKeyPlatformUsage struct {
	ent.Schema
}

func (APIKeyPlatformUsage) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "api_key_platform_usages"},
	}
}

func (APIKeyPlatformUsage) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
	}
}

func (APIKeyPlatformUsage) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("api_key_id"),
		field.String("platform").
			MaxLen(32).
			NotEmpty().
			Validate(func(s string) error {
				// 注意：平台列表的单一权威源为 service.AllowedQuotaPlatforms；
				// 此处为 ent 构建期约束，需与 service.AllowedQuotaPlatforms 保持同步。
				switch s {
				case "anthropic", "openai", "gemini", "antigravity", "grok",
					"kimi", "zhipu", "deepseek", "minimax", "opencode_go":
					return nil
				default:
					return fmt.Errorf("platform %q is not allowed", s)
				}
			}),

		// 该来源上累计消费（USD），对应 platform_limits[platform].quota
		field.Float("quota_used").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}).
			Default(0),

		// 滚动窗口用量（USD），语义与 api_keys.usage_5h/1d/7d 完全一致
		field.Float("usage_5h").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}).
			Default(0),
		field.Float("usage_1d").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}).
			Default(0),
		field.Float("usage_7d").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}).
			Default(0),

		// 窗口起点（NULL = 尚未初始化，IsWindowExpired 视为已过期）
		field.Time("window_5h_start").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("window_1d_start").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("window_7d_start").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (APIKeyPlatformUsage) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("api_key", APIKey.Type).
			Ref("platform_usages").
			Field("api_key_id").
			Unique().
			Required(),
	}
}

func (APIKeyPlatformUsage) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("api_key_id", "platform").Unique(),
		index.Fields("api_key_id"),
	}
}
