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

type OrganizationMemberSpendLimit struct{ ent.Schema }

func (OrganizationMemberSpendLimit) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "organization_member_spend_limits"}}
}

func (OrganizationMemberSpendLimit) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("organization_id").Immutable(),
		field.Int64("member_user_id").Optional().Nillable().Immutable(),
		field.Float("daily_limit_usd").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "numeric(20,10)"}),
		field.Float("monthly_limit_usd").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "numeric(20,10)"}),
		field.Bool("alert_enabled").Default(false),
		field.Float("alert_threshold_pct").Default(80).SchemaType(map[string]string{dialect.Postgres: "numeric(5,2)"}),
		field.Strings("additional_recipients").Default([]string{}).SchemaType(map[string]string{dialect.Postgres: "text[]"}),
		field.Int64("revision").Default(1),
		field.Time("created_at").Immutable().Default(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (OrganizationMemberSpendLimit) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("organization_id").Unique().Annotations(entsql.IndexWhere("member_user_id IS NULL")),
		index.Fields("organization_id", "member_user_id").Unique().Annotations(entsql.IndexWhere("member_user_id IS NOT NULL")),
		index.Fields("member_user_id").Annotations(entsql.IndexWhere("member_user_id IS NOT NULL")),
	}
}
