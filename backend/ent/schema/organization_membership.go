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

type OrganizationMembership struct{ ent.Schema }

func (OrganizationMembership) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "organization_memberships"}}
}

func (OrganizationMembership) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("organization_id").Immutable(),
		field.Int64("user_id").Immutable().Unique(),
		field.String("role").MaxLen(16).Immutable(),
		field.String("status").MaxLen(16).Default("active"),
		field.Int64("authz_generation").Default(1),
		field.Time("archived_at").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("created_at").Immutable().Default(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (OrganizationMembership) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("organization_id", "status"),
		index.Fields("organization_id").Unique().Annotations(entsql.IndexWhere("role = 'owner'")),
	}
}
