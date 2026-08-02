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

type OrganizationFinancialLedger struct{ ent.Schema }

func (OrganizationFinancialLedger) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "organization_financial_ledger"}}
}

func (OrganizationFinancialLedger) Fields() []ent.Field {
	return []ent.Field{
		field.String("idempotency_key").MaxLen(160).Immutable().Unique(),
		field.String("kind").MaxLen(32).Immutable(),
		field.Int64("organization_id").Optional().Nillable().Immutable(),
		field.Int64("application_id").Optional().Nillable().Immutable(),
		field.Int64("actor_user_id").Immutable(),
		field.Int64("source_user_id").Optional().Nillable().Immutable(),
		field.Int64("destination_user_id").Optional().Nillable().Immutable(),
		field.Float("amount").Immutable().SchemaType(map[string]string{dialect.Postgres: "numeric(20,8)"}),
		field.String("currency").MaxLen(8).Immutable().Default("USD"),
		field.Float("source_balance_after").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "numeric(20,8)"}),
		field.Float("destination_balance_after").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "numeric(20,8)"}),
		field.Time("created_at").Immutable().Default(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (OrganizationFinancialLedger) Indexes() []ent.Index {
	return []ent.Index{index.Fields("organization_id", "created_at")}
}
