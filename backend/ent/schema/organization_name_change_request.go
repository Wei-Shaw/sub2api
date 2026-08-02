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

type OrganizationNameChangeRequest struct{ ent.Schema }

func (OrganizationNameChangeRequest) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "organization_name_change_requests"}}
}

func (OrganizationNameChangeRequest) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("organization_id").Immutable(),
		field.Int64("applicant_user_id").Immutable(),
		field.String("old_name").MaxLen(255).Immutable(),
		field.String("new_name").MaxLen(255).Immutable(),
		field.String("normalized_name").MaxLen(255).Immutable(),
		field.String("status").MaxLen(16).Default("pending"),
		field.Int64("reviewer_user_id").Optional().Nillable(),
		field.String("review_reason").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "text"}),
		field.Time("decided_at").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("created_at").Immutable().Default(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (OrganizationNameChangeRequest) Indexes() []ent.Index {
	return []ent.Index{index.Fields("organization_id").Unique().Annotations(entsql.IndexWhere("status = 'pending'"))}
}
