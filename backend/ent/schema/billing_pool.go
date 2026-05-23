package schema

import (
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"
	"github.com/Wei-Shaw/sub2api/internal/domain"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type BillingPool struct {
	ent.Schema
}

func (BillingPool) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "billing_pools"},
	}
}

func (BillingPool) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
		mixins.SoftDeleteMixin{},
	}
}

func (BillingPool) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").MaxLen(100).NotEmpty(),
		field.String("code").MaxLen(100).NotEmpty(),
		field.String("description").Optional().Nillable(),
		field.String("status").MaxLen(20).Default(domain.StatusActive),
		field.String("platform_scope").MaxLen(32).Default("same_platform"),
		field.Bool("allow_user_reorder").Default(false),
		field.Bool("require_primary_subscription").Default(true),
		field.Bool("allow_balance_fallback").Default(true),
	}
}

func (BillingPool) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("members", BillingPoolGroup.Type),
		edge.To("api_keys", APIKey.Type),
	}
}

func (BillingPool) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("status"),
		index.Fields("deleted_at"),
		index.Fields("platform_scope"),
	}
}
