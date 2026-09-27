package schema

import (
	"fmt"

	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

var studentVerificationStatuses = map[string]struct{}{
	"active":  {},
	"expired": {},
	"revoked": {},
}

func validateStudentVerificationStatus(value string) error {
	if _, ok := studentVerificationStatuses[value]; ok {
		return nil
	}
	return fmt.Errorf("invalid student verification status %q", value)
}

// StudentVerification records a user's verified student eligibility claim.
// A school email can be claimed by at most one account; the unique index on
// email is the authoritative guard against sharing one email across users.
type StudentVerification struct {
	ent.Schema
}

func (StudentVerification) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "student_verifications"},
	}
}

func (StudentVerification) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
	}
}

func (StudentVerification) Fields() []ent.Field {
	return []ent.Field{
		// user_id is nullable + ON DELETE SET NULL: deleting a user must NOT
		// release the school-email claim; only an explicit admin delete does.
		field.Int64("user_id").
			Optional().
			Nillable(),
		field.String("email").
			NotEmpty().
			SchemaType(map[string]string{dialect.Postgres: "text"}),
		field.String("email_raw").
			NotEmpty().
			SchemaType(map[string]string{dialect.Postgres: "text"}),
		field.String("status").
			MaxLen(20).
			NotEmpty().
			Default("active").
			Validate(validateStudentVerificationStatus),
		field.Time("verified_at").
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("expires_at").
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("revoked_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Int64("revoked_by").
			Optional().
			Nillable(),
		field.String("revoke_reason").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "text"}),
		field.Float("rebate_rate_applied").
			Optional().
			Nillable(),
		field.Float("previous_rebate_rate").
			Optional().
			Nillable(),
		field.JSON("granted_group_ids", []int64{}).
			Default(func() []int64 { return []int64{} }).
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}),
	}
}

func (StudentVerification) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).
			Ref("student_verification").
			Field("user_id").
			Unique().
			Annotations(entsql.Annotation{OnDelete: entsql.SetNull}),
	}
}

func (StudentVerification) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("email").Unique(),
		index.Fields("user_id").Unique(),
		index.Fields("status", "expires_at"),
	}
}
