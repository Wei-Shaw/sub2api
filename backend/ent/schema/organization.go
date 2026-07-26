package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type Organization struct{ ent.Schema }

func (Organization) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "organizations"}}
}

func (Organization) Fields() []ent.Field {
	return []ent.Field{
		field.String("account_id").MaxLen(16).Immutable().Unique(),
		field.Int64("owner_user_id").Immutable(),
		field.String("name").MaxLen(255),
		field.String("normalized_name").MaxLen(255),
		field.String("status").MaxLen(16).Default("active"),
		field.Int("member_limit").Default(20),
		field.Time("effective_at").Immutable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("created_at").Immutable().Default(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (Organization) Indexes() []ent.Index {
	return []ent.Index{index.Fields("owner_user_id"), index.Fields("status")}
}
