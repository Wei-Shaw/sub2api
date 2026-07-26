package schema

import (
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// CustomDomain is a user-owned hostname accepted by the API gateway.
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
		field.String("domain").
			MaxLen(253).
			NotEmpty().
			Validate(validateCustomDomainName),
		field.String("status").
			MaxLen(32).
			Default("pending_dns").
			Validate(func(value string) error {
				switch value {
				case "pending_dns", "active", "disabled":
					return nil
				default:
					return fmt.Errorf("must be one of pending_dns, active, disabled")
				}
			}),
		field.Bool("all_users").Default(false),
		field.String("verification_token").MaxLen(128).NotEmpty(),
		field.String("verification_txt_name").MaxLen(253).NotEmpty(),
		field.String("verification_txt_value").MaxLen(256).NotEmpty(),
		field.String("cname_target").
			MaxLen(253).
			Optional().
			Nillable().
			Validate(validateOptionalCustomDomainName),
		field.Time("verified_at").Optional().Nillable(),
		field.Time("last_checked_at").Optional().Nillable(),
		field.String("last_error").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "text"}),
		field.Time("disabled_at").Optional().Nillable(),
		field.String("disabled_reason").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "text"}),
	}
}

func (CustomDomain) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).
			Ref("custom_domains").
			Field("user_id").
			Required().
			Unique(),
		edge.From("authorized_users", User.Type).
			Ref("authorized_custom_domains").
			Through("custom_domain_users", CustomDomainUser.Type),
	}
}

func (CustomDomain) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id"),
		index.Fields("status"),
		index.Fields("all_users"),
		index.Fields("deleted_at"),
	}
}

func validateCustomDomainName(value string) error {
	if strings.TrimSpace(value) != value || value == "" {
		return fmt.Errorf("domain must not be blank or padded")
	}
	if strings.Contains(value, "--") {
		return fmt.Errorf("domain labels must not contain --")
	}
	return nil
}

func validateOptionalCustomDomainName(value string) error {
	if value == "" {
		return nil
	}
	return validateCustomDomainName(value)
}
