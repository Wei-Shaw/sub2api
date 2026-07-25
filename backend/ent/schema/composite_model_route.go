package schema

import (
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"
	"github.com/Wei-Shaw/sub2api/internal/domain"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// CompositeModelRoute holds model routing aliases for composite groups.
type CompositeModelRoute struct {
	ent.Schema
}

func (CompositeModelRoute) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "composite_model_routes"},
	}
}

func (CompositeModelRoute) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
		mixins.SoftDeleteMixin{},
	}
}

func (CompositeModelRoute) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("group_id"),
		field.String("public_model").
			MaxLen(200).
			NotEmpty().
			Comment("Client-facing model identifier or prefix."),
		field.String("match_type").
			MaxLen(20).
			Default("exact").
			Comment("exact or prefix."),
		field.String("target_platform").
			MaxLen(50).
			Default(domain.PlatformOpenAI).
			Comment("Concrete provider platform."),
		field.String("upstream_model").
			MaxLen(200).
			Default("").
			Comment("Provider model identifier; empty means public_model."),
		field.String("endpoint").
			MaxLen(50).
			Default("any").
			Comment("Endpoint scope such as any, messages, responses, chat_completions."),
		field.Int("priority").
			Default(100).
			Comment("Lower values win within the same match strength."),
		field.Bool("enabled").
			Default(true),
		field.String("notes").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "text"}),
	}
}

func (CompositeModelRoute) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("group", Group.Type).
			Unique().
			Required().
			Field("group_id"),
	}
}

func (CompositeModelRoute) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("group_id"),
		index.Fields("group_id", "enabled"),
		index.Fields("group_id", "endpoint"),
		index.Fields("group_id", "target_platform"),
		index.Fields("deleted_at"),
		index.Fields("priority"),
	}
}
