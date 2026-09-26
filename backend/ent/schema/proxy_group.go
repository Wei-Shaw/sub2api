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

// ProxyGroup stores a reusable set of proxy endpoints.
type ProxyGroup struct {
	ent.Schema
}

func (ProxyGroup) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "proxy_groups"}}
}

func (ProxyGroup) Mixin() []ent.Mixin {
	return []ent.Mixin{mixins.TimeMixin{}, mixins.SoftDeleteMixin{}}
}

func (ProxyGroup) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").MaxLen(100).NotEmpty(),
		field.String("description").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "text"}),
		field.String("status").MaxLen(20).Default("active"),
	}
}

func (ProxyGroup) Edges() []ent.Edge {
	return []ent.Edge{edge.From("accounts", Account.Type).Ref("proxy_group")}
}

func (ProxyGroup) Indexes() []ent.Index {
	return []ent.Index{index.Fields("status"), index.Fields("deleted_at")}
}
