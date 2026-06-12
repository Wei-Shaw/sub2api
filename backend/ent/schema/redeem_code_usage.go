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

// RedeemCodeUsage holds the schema definition for the redeem usage ledger.
type RedeemCodeUsage struct {
	ent.Schema
}

func (RedeemCodeUsage) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "redeem_code_usages"},
	}
}

func (RedeemCodeUsage) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("redeem_code_id"),
		field.String("usage_scope").
			SchemaType(map[string]string{dialect.Postgres: "text"}),
		field.Int64("user_id"),
		field.String("code_snapshot").
			MaxLen(32),
		field.String("type_snapshot").
			MaxLen(20),
		field.Float("value_snapshot").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}).
			Default(0),
		field.Int64("group_id_snapshot").
			Optional().
			Nillable(),
		field.Int("validity_days_snapshot").
			Default(30),
		field.Time("used_at").
			Default(time.Now).
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.JSON("metadata", map[string]any{}).
			Optional().
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}),
	}
}

func (RedeemCodeUsage) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("redeem_code", RedeemCode.Type).
			Ref("usages").
			Field("redeem_code_id").
			Required().
			Unique(),
		edge.From("user", User.Type).
			Ref("redeem_code_usages").
			Field("user_id").
			Required().
			Unique(),
	}
}

func (RedeemCodeUsage) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("redeem_code_id"),
		index.Fields("user_id"),
		index.Fields("used_at"),
		index.Fields("usage_scope", "user_id").Unique(),
	}
}
