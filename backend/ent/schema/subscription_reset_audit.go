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

// SubscriptionResetAudit holds audit rows for user-triggered subscription quota resets.
type SubscriptionResetAudit struct {
	ent.Schema
}

func (SubscriptionResetAudit) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "subscription_reset_audits"},
	}
}

func (SubscriptionResetAudit) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("subscription_id"),
		field.Int64("user_id"),
		field.Int64("group_id"),
		field.Int64("operator_id"),
		field.String("operator_type").MaxLen(20).Default("user"),
		field.Int("deducted_seconds").Default(86400),
		field.Time("before_expires_at").SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("after_expires_at").SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Float("before_daily_usage_usd").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,10)"}).
			Default(0),
		field.Float("after_daily_usage_usd").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,10)"}).
			Default(0),
		field.Time("before_daily_window_start").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("after_daily_window_start").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("created_at").
			Immutable().
			Default(time.Now).
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (SubscriptionResetAudit) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("subscription_id"),
		index.Fields("user_id"),
		index.Fields("group_id"),
		index.Fields("operator_id"),
		index.Fields("created_at"),
	}
}
