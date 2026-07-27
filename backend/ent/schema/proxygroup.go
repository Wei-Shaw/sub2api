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

// ProxyGroup holds the schema definition for a proxy pool/group.
//
// Accounts may bind to a group via proxy_group_id. At hydration time the
// service selects one healthy member proxy by strategy and fills account.Proxy.
// See openspec/changes/add-proxy-group-pool.
type ProxyGroup struct {
	ent.Schema
}

func (ProxyGroup) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "proxy_groups"},
	}
}

func (ProxyGroup) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
		mixins.SoftDeleteMixin{},
	}
}

func (ProxyGroup) Fields() []ent.Field {
	return []ent.Field{
		// 唯一约束通过部分索引实现（WHERE deleted_at IS NULL），支持软删除后重用
		// 见迁移文件 191_add_proxy_groups.sql
		field.String("name").
			MaxLen(100).
			NotEmpty(),
		field.String("description").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "text"}),
		field.String("strategy").
			MaxLen(20).
			Default("round_robin").
			Comment("Selection strategy: round_robin | random | sticky."),
		field.Bool("sticky_by_account").
			Default(false).
			Comment("When true, selection is sticky by account id (hash modulo)."),
		field.String("status").
			MaxLen(20).
			Default("active"),
	}
}

func (ProxyGroup) Edges() []ent.Edge {
	return []ent.Edge{
		// proxies: 归属本组的代理（一对多，proxies.group_id）
		edge.To("proxies", Proxy.Type),
		// accounts: 绑定本组的账户（反向边，accounts.proxy_group_id）
		edge.From("accounts", Account.Type).
			Ref("proxy_group"),
	}
}

func (ProxyGroup) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("status"),
		index.Fields("deleted_at"),
	}
}
