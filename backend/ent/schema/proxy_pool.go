package schema

import (
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// ProxyPool holds the schema definition for the ProxyPool entity.
type ProxyPool struct {
	ent.Schema
}

func (ProxyPool) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "proxy_pools"},
	}
}

func (ProxyPool) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
		mixins.SoftDeleteMixin{},
	}
}

func (ProxyPool) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").
			MaxLen(100).
			NotEmpty(),
		field.String("description").
			Optional().
			Nillable(),
		field.String("status").
			MaxLen(20).
			Default("active"),
		field.Int("health_interval_seconds").
			Default(300).
			Comment("Health probe interval in seconds."),
		field.Int("failure_threshold").
			Default(2).
			Comment("Consecutive probe failures before a proxy is marked unhealthy."),
		field.Bool("auto_rebind").
			Default(true).
			Comment("Automatically rebind accounts from unhealthy pool proxies to a healthy one."),
	}
}

// Edges defines the entity relationships for ProxyPool.
func (ProxyPool) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("proxies", Proxy.Type).
			Ref("pool"),
	}
}

func (ProxyPool) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("status"),
		index.Fields("deleted_at"),
	}
}
