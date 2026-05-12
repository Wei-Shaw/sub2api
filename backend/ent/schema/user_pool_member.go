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

type UserPoolMember struct {
	ent.Schema
}

func (UserPoolMember) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "user_pool_members"},
		field.ID("pool_id", "user_id"),
	}
}

func (UserPoolMember) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("pool_id"),
		field.Int64("user_id"),
		field.Time("created_at").
			Immutable().
			Default(time.Now).
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (UserPoolMember) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("pool", UserPool.Type).
			Unique().
			Required().
			Field("pool_id"),
		edge.To("user", User.Type).
			Unique().
			Required().
			Field("user_id"),
	}
}

func (UserPoolMember) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id"),
	}
}
