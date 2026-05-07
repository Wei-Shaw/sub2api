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

var authProviderTypes = map[string]struct{}{
	"email":   {},
	"github":  {},
	"google":  {},
	"linuxdo": {},
	"oidc":    {},
	"wechat":  {},
}

func validateAuthProviderType(value string) error {
	if _, ok := authProviderTypes[value]; ok {
		return nil
	}
	return fmt.Errorf("invalid auth provider type %q", value)
}

// AuthIdentity stores the canonical login identity for an account.
type AuthIdentity struct {
	ent.Schema
}

func (AuthIdentity) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "auth_identities"},
	}
}

func (AuthIdentity) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
	}
}

func (AuthIdentity) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("user_id"),
		field.String("provider_type").
			MaxLen(20).
			NotEmpty().
			Validate(validateAuthProviderType),
		field.String("provider_key").
			NotEmpty().
			SchemaType(map[string]string{dialect.Postgres: "text"}),
		field.String("provider_subject").
			NotEmpty().
			SchemaType(map[string]string{dialect.Postgres: "text"}),
		field.Time("verified_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.String("issuer").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "text"}),
		field.JSON("metadata", map[string]any{}).
			Default(func() map[string]any { return map[string]any{} }).
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}),
	}
}

func (AuthIdentity) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).
			Ref("auth_identities").
			Field("user_id").
			Required().
			Unique(),
		edge.To("channels", AuthIdentityChannel.Type).
			Annotations(entsql.OnDelete(entsql.Cascade)),
		edge.To("adoption_decisions", IdentityAdoptionDecision.Type),
	}
}

func (AuthIdentity) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("provider_type", "provider_key", "provider_subject").Unique(),
		index.Fields("user_id"),
		index.Fields("user_id", "provider_type"),
	}
}
