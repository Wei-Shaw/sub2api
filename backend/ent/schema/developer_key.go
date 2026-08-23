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

// DeveloperKey is a user-owned credential restricted to developer APIs.
type DeveloperKey struct {
	ent.Schema
}

func (DeveloperKey) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "developer_keys"},
	}
}

func (DeveloperKey) Mixin() []ent.Mixin {
	return []ent.Mixin{mixins.TimeMixin{}}
}

func (DeveloperKey) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("user_id"),
		field.String("name").MaxLen(100).NotEmpty(),
		field.String("key_prefix").MaxLen(24).NotEmpty(),
		field.String("key_hash").MaxLen(64).NotEmpty().Unique(),
		field.Time("last_used_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (DeveloperKey) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).
			Ref("developer_keys").
			Field("user_id").
			Unique().
			Required(),
	}
}

func (DeveloperKey) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id", "created_at"),
	}
}
