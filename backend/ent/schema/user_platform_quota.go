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
	"github.com/Wei-Shaw/sub2api/internal/domain"
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
				// 平台白名单为平台清单（domain/platforms.go）中的全部具体平台，
				// 与 service.AllowedQuotaPlatforms 同源；数据库不再维护 CHECK 约束。
				if !domain.IsConcretePlatform(s) {
					return fmt.Errorf("platform %q is not allowed", s)
				}
				return nil
			}),

		// 日 / 周 / 月 USD 上限：
		//   nil / not set → 无限额（完全放行）
		//   0            → 完全禁用（任何请求都会被拒绝，因为 usage >= 0 恒成立）
		//   > 0          → USD 限额上限
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
