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

type UserPoolGroupGrant struct {
	ent.Schema
}

func (UserPoolGroupGrant) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "user_pool_group_grants"},
		field.ID("pool_id", "group_id"),
	}
}

func (UserPoolGroupGrant) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("pool_id"),
		field.Int64("group_id"),
		field.Float("rate_multiplier").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "decimal(10,4)"}),
		field.Int("rpm_override").
			Optional().
			Nillable(),
		field.Time("created_at").
			Immutable().
			Default(time.Now).
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now).
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (UserPoolGroupGrant) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("pool", UserPool.Type).
			Unique().
			Required().
			Field("pool_id"),
		edge.To("group", Group.Type).
			Unique().
			Required().
			Field("group_id"),
	}
}

func (UserPoolGroupGrant) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("group_id"),
	}
}
