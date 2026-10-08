// Package schema defines Ent ORM database schemas.
package schema

import (
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// CustomDomain stores user-owned API hostnames after DNS verification.
type CustomDomain struct {
	ent.Schema
}

func (CustomDomain) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "custom_domains"},
	}
}

func (CustomDomain) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
		mixins.SoftDeleteMixin{},
	}
}

func (CustomDomain) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("user_id"),
		field.Bool("all_users").
			Default(false),
		field.String("domain").
			MaxLen(253).
			NotEmpty(),
		field.String("status").
			MaxLen(32).
			Default("pending_dns"),
		field.String("verification_token").
			MaxLen(128).
			NotEmpty(),
		field.String("verification_txt_name").
			MaxLen(253).
			NotEmpty(),
		field.String("verification_txt_value").
			MaxLen(256).
			NotEmpty(),
		field.String("cname_target").
			MaxLen(253).
			Optional().
			Nillable(),
		field.String("last_error").
			SchemaType(map[string]string{dialect.Postgres: "text"}).
			Optional().
			Nillable(),
		field.Time("verified_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("last_checked_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("disabled_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.String("disabled_reason").
			SchemaType(map[string]string{dialect.Postgres: "text"}).
			Optional().
			Nillable(),
	}
}

func (CustomDomain) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).
			Ref("custom_domains").
			Field("user_id").
			Required().
			Unique(),
		edge.To("authorized_users", User.Type).
			Through("custom_domain_users", CustomDomainUser.Type),
	}
}

func (CustomDomain) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id"),
		index.Fields("all_users"),
		index.Fields("status"),
		index.Fields("domain"),
	}
}
