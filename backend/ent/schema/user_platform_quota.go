package schema

import (
	"fmt"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"
)

// UserPlatformQuota holds the schema definition for per-user per-platform quota.
type UserPlatformQuota struct {
	ent.Schema
}

func (UserPlatformQuota) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "user_platform_quotas"},
	}
}

func (UserPlatformQuota) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
		mixins.SoftDeleteMixin{},
	}
}

func (UserPlatformQuota) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("user_id"),
		field.String("platform").
			MaxLen(32).
			NotEmpty().
			Validate(func(s string) error {
				// 注意：平台列表的单一权威源为 service.AllowedQuotaPlatforms；
				// 此处为 ent 构建期约束，需与 service.AllowedQuotaPlatforms 保持同步。
				switch s {
				case "anthropic", "openai", "gemini", "antigravity", "grok":
					return nil
				default:
					return fmt.Errorf("platform %q is not allowed", s)
				}
			}),

		// 5 小时 / 日 / 周 / 月 USD 上限：
		//   nil / not set → 无限额（完全放行）
		//   0            → 完全禁用（任何请求都会被拒绝，因为 usage >= 0 恒成立）
		//   > 0          → USD 限额上限
		field.Float("five_hour_limit_usd").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,10)"}),
		field.Float("daily_limit_usd").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,10)"}),
		field.Float("weekly_limit_usd").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,10)"}),
		field.Float("monthly_limit_usd").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,10)"}),

		// 当前窗口已用量（USD，preflight 时与 limit 比较）
		field.Float("five_hour_usage_usd").
			Default(0).
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,10)"}),
		field.Float("daily_usage_usd").
			Default(0).
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,10)"}),
		field.Float("weekly_usage_usd").
			Default(0).
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,10)"}),
		field.Float("monthly_usage_usd").
			Default(0).
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,10)"}),

		// 窗口起点（NULL = 首次还未初始化，由 InitWindowStarts 用 COALESCE 兜底）
		field.Time("five_hour_window_start").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("daily_window_start").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("weekly_window_start").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("monthly_window_start").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),

		// 管理员重置代次。缓存快照仅能覆盖相同代次的数据库记录。
		field.Int64("reset_generation").
			Default(0).
			NonNegative(),
	}
}

func (UserPlatformQuota) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).
			Ref("platform_quotas").
			Field("user_id").
			Unique().
			Required(),
	}
}

func (UserPlatformQuota) Indexes() []ent.Index {
	return []ent.Index{
		// 软删除友好：只对未删记录唯一
		index.Fields("user_id", "platform").
			Unique().
			Annotations(entsql.IndexWhere("deleted_at IS NULL")),
		index.Fields("user_id"),
	}
}
