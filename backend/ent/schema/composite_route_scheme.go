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

// CompositeRouteScheme is a reusable named set of composite model routes.
// Composite groups bind to one scheme instead of owning routes directly.
type CompositeRouteScheme struct {
	ent.Schema
}

func (CompositeRouteScheme) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "composite_route_schemes"},
	}
}

func (CompositeRouteScheme) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
		mixins.SoftDeleteMixin{},
	}
}

func (CompositeRouteScheme) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").
			MaxLen(100).
			NotEmpty().
			Comment("Admin-facing scheme name."),
		field.String("description").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "text"}),
	}
}

func (CompositeRouteScheme) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("routes", CompositeModelRoute.Type),
		edge.From("groups", Group.Type).
			Ref("composite_route_scheme"),
	}
}

func (CompositeRouteScheme) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("deleted_at"),
	}
}
