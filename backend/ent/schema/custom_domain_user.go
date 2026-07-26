package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// CustomDomainUser stores explicit user authorization for a custom domain.
type CustomDomainUser struct {
	ent.Schema
}

func (CustomDomainUser) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "custom_domain_users"},
		field.ID("user_id", "custom_domain_id"),
	}
}

func (CustomDomainUser) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("custom_domain_id"),
		field.Int64("user_id"),
		field.Time("created_at").
			Immutable().
			Default(time.Now).
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (CustomDomainUser) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("custom_domain", CustomDomain.Type).
			Unique().
			Required().
			Field("custom_domain_id"),
		edge.To("user", User.Type).
			Unique().
			Required().
			Field("user_id"),
	}
}

func (CustomDomainUser) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id"),
	}
}
