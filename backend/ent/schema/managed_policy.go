package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
)

type ManagedPolicy struct{ ent.Schema }

func (ManagedPolicy) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "managed_policies"}}
}

func (ManagedPolicy) Fields() []ent.Field {
	return []ent.Field{
		field.String("policy_key").MaxLen(128).Immutable().Unique(),
		field.String("display_name").MaxLen(128),
		field.String("policy_type").MaxLen(32).Default("system"),
		field.String("description").SchemaType(map[string]string{dialect.Postgres: "text"}),
		field.Int("version").Default(1),
		field.Time("created_at").Immutable().Default(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}
