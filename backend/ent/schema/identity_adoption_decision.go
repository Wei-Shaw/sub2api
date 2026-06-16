package schema

import (
	"time"

	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// IdentityAdoptionDecision stores the one-time profile adoption choice captured during a pending auth flow.
type IdentityAdoptionDecision struct {
	ent.Schema
}

func (IdentityAdoptionDecision) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "identity_adoption_decisions"},
	}
}

func (IdentityAdoptionDecision) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
	}
}

func (IdentityAdoptionDecision) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("pending_auth_session_id"),
		field.Int64("identity_id").
			Optional().
			Nillable(),
		field.Bool("adopt_display_name").
			Default(false),
		field.Bool("adopt_avatar").
			Default(false),
		field.Time("decided_at").
			Immutable().
			Default(time.Now).
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (IdentityAdoptionDecision) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("pending_auth_session", PendingAuthSession.Type).
			Ref("adoption_decision").
			Field("pending_auth_session_id").
			Required().
			Unique(),
		edge.From("identity", AuthIdentity.Type).
			Ref("adoption_decisions").
			Field("identity_id").
			Unique(),
	}
}

func (IdentityAdoptionDecision) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("pending_auth_session_id").Unique(),
		index.Fields("identity_id"),
	}
}
