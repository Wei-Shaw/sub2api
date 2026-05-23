package schema

import (
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type BillingPoolGroup struct {
	ent.Schema
}

func (BillingPoolGroup) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "billing_pool_groups"},
	}
}

func (BillingPoolGroup) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
		mixins.SoftDeleteMixin{},
	}
}

func (BillingPoolGroup) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("billing_pool_id"),
		field.Int64("group_id"),
		field.Int("chain_order").Default(0),
		field.Bool("can_be_primary").Default(true),
		field.Bool("can_be_fallback").Default(true),
	}
}

func (BillingPoolGroup) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("billing_pool", BillingPool.Type).
			Ref("members").
			Field("billing_pool_id").
			Unique().
			Required(),
		edge.From("group", Group.Type).
			Ref("billing_pool_memberships").
			Field("group_id").
			Unique().
			Required(),
	}
}

func (BillingPoolGroup) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("billing_pool_id"),
		index.Fields("group_id"),
		index.Fields("chain_order"),
		index.Fields("deleted_at"),
	}
}
