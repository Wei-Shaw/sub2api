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

type MemberPolicyAttachment struct{ ent.Schema }

func (MemberPolicyAttachment) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "member_policy_attachments"}}
}

func (MemberPolicyAttachment) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("organization_id").Immutable(),
		field.Int64("membership_id").Immutable(),
		field.Int64("policy_id").Immutable(),
		field.Int("policy_version").Immutable(),
		field.Int64("attached_by_user_id").Immutable(),
		field.Int64("detached_by_user_id").Optional().Nillable(),
		field.Time("detached_at").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("created_at").Immutable().Default(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (MemberPolicyAttachment) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("membership_id", "policy_id").Unique().Annotations(entsql.IndexWhere("detached_at IS NULL")),
		index.Fields("organization_id"),
	}
}
