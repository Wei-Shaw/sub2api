package schema

import (
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// UserUsageLimitRule 用户配额规则（feature issue #1750）
// 关联到 users；一个用户可以配置多条规则，每条规则绑定一组 group_ids +
// 日限额。应用层保证同一用户内 group_ids 互不重叠。
type UserUsageLimitRule struct {
	ent.Schema
}

func (UserUsageLimitRule) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "user_usage_limit_rules"},
	}
}

func (UserUsageLimitRule) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
	}
}

func (UserUsageLimitRule) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("user_id"),
		field.JSON("group_ids", []int64{}).
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}).
			Comment("命中分组 ID 列表（非空，元素去重）；规则间 group_ids 禁止重叠"),
		field.Float("daily_limit_usd").
			SchemaType(map[string]string{dialect.Postgres: "numeric(20,8)"}).
			Comment("本规则下每日限额（USD）"),
		field.String("period").
			MaxLen(16).
			Default("daily").
			Comment("扩展占位：daily / weekly / monthly；首版只支持 daily"),
	}
}

func (UserUsageLimitRule) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).
			Ref("usage_limit_rules").
			Field("user_id").
			Unique().
			Required(),
	}
}

func (UserUsageLimitRule) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id"),
	}
}
