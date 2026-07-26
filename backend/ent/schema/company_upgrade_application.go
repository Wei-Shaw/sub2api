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

type CompanyUpgradeApplication struct{ ent.Schema }

func (CompanyUpgradeApplication) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "company_upgrade_applications"}}
}

func (CompanyUpgradeApplication) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("applicant_user_id").Immutable(),
		field.String("requested_name").MaxLen(255).Immutable(),
		field.String("normalized_name").MaxLen(255).Immutable(),
		field.String("status").MaxLen(16).Default("pending"),
		field.Float("fee_amount").Immutable().SchemaType(map[string]string{dialect.Postgres: "numeric(20,8)"}),
		field.String("fee_currency").MaxLen(8).Immutable().Default("USD"),
		field.String("idempotency_key").MaxLen(128).Immutable(),
		field.Int64("reviewer_user_id").Optional().Nillable(),
		field.String("review_reason").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "text"}),
		field.Int64("organization_id").Optional().Nillable(),
		field.Time("decided_at").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("created_at").Immutable().Default(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (CompanyUpgradeApplication) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("status", "created_at"),
		index.Fields("applicant_user_id", "idempotency_key").Unique(),
		index.Fields("applicant_user_id").Unique().Annotations(entsql.IndexWhere("status = 'pending'")),
	}
}
