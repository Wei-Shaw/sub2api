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

type ManagedPolicyAction struct{ ent.Schema }

func (ManagedPolicyAction) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "managed_policy_actions"}}
}

func (ManagedPolicyAction) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("policy_id").Immutable(),
		field.String("action").MaxLen(160).Immutable(),
		field.Time("created_at").Immutable().Default(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (ManagedPolicyAction) Indexes() []ent.Index {
	return []ent.Index{index.Fields("policy_id", "action").Unique()}
}
