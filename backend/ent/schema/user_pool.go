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

// UserPool holds the schema definition for the user_pools entity.
type UserPool struct {
	ent.Schema
}

func (UserPool) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "user_pools"},
	}
}

func (UserPool) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
		mixins.SoftDeleteMixin{},
	}
}

func (UserPool) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").
			MaxLen(100).
			NotEmpty(),
		field.String("description").
			Optional().
			Nillable().
			SchemaType(map[string]string{"postgres": "text"}),
		field.String("status").
			MaxLen(20).
			Default("active"),
	}
}

func (UserPool) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("users", User.Type).
			Through("members", UserPoolMember.Type),
		edge.To("groups", Group.Type).
			Through("group_grants", UserPoolGroupGrant.Type),
	}
}

func (UserPool) Indexes() []ent.Index {
	return []ent.Index{
		// Partial unique index: only active (non-deleted) pools must have unique names
		index.Fields("name").
			Unique().
			Annotations(entsql.IndexWhere("deleted_at IS NULL")),
		index.Fields("status"),
		index.Fields("deleted_at"),
	}
}
