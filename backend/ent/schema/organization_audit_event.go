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

type OrganizationAuditEvent struct{ ent.Schema }

func (OrganizationAuditEvent) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "organization_audit_events"}}
}

func (OrganizationAuditEvent) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("organization_id").Optional().Nillable().Immutable(),
		field.Int64("actor_user_id").Optional().Nillable().Immutable(),
		field.Int64("subject_user_id").Optional().Nillable().Immutable(),
		field.String("action").MaxLen(128).Immutable(),
		field.String("result").MaxLen(32).Immutable(),
		field.String("correlation_id").MaxLen(128).Optional().Nillable().Immutable(),
		field.JSON("metadata", map[string]any{}).Optional().Immutable(),
		field.Time("created_at").Immutable().Default(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (OrganizationAuditEvent) Indexes() []ent.Index {
	return []ent.Index{index.Fields("organization_id", "created_at")}
}
