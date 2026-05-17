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

// BalancePackage holds a purchasable balance/credit top-up package maintained by admins.
// It is intentionally separate from SubscriptionPlan: subscriptions grant group access,
// while balance packages only define how much ledger balance a paid top-up credits.
type BalancePackage struct {
	ent.Schema
}

func (BalancePackage) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "balance_packages"},
	}
}

func (BalancePackage) Fields() []ent.Field {
	return []ent.Field{
		field.String("code").MaxLen(64).NotEmpty().Unique(),
		field.String("label").MaxLen(100).NotEmpty(),
		field.String("description").SchemaType(map[string]string{dialect.Postgres: "text"}).Default(""),
		field.Float("amount_ledger").SchemaType(map[string]string{dialect.Postgres: "decimal(20,2)"}),
		field.Int64("actual_credits").Default(0),
		field.String("credit_unit").MaxLen(32).Default("tokens"),
		field.Int64("group_id").Optional().Nillable(),
		field.JSON("currency_overrides", map[string]float64{}).
			Optional().
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}).
			Comment("Per-currency display/payment price overrides; key=ISO currency code, value=amount in that currency. When set, this amount is charged instead of FX-converting the ledger amount."),
		field.String("badge").MaxLen(100).Default(""),
		field.Bool("popular").Default(false),
		field.Bool("for_sale").Default(true),
		field.Int("sort_order").Default(0),
		field.Time("created_at").Immutable().Default(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (BalancePackage) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("code").Unique(),
		index.Fields("for_sale"),
		index.Fields("group_id"),
		index.Fields("sort_order"),
	}
}
