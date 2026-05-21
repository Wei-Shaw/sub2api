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

// PlaygroundImageTask holds the schema definition for a user's Playground image task.
type PlaygroundImageTask struct {
	ent.Schema
}

func (PlaygroundImageTask) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "playground_image_tasks"},
	}
}

func (PlaygroundImageTask) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
		mixins.SoftDeleteMixin{},
	}
}

func (PlaygroundImageTask) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("user_id"),
		field.Int64("api_key_id").
			Optional().
			Nillable(),
		field.String("model").
			MaxLen(100).
			Default(""),
		field.String("prompt").
			SchemaType(map[string]string{dialect.Postgres: "text"}).
			Default(""),
		field.String("quality").
			MaxLen(20).
			Default(""),
		field.String("size").
			MaxLen(50).
			Default(""),
		field.Int("n").
			Default(1),
		field.String("endpoint").
			MaxLen(100).
			Default("/v1/images/generations"),
		field.String("status").
			MaxLen(20).
			Default("pending"),
		field.JSON("request", map[string]any{}).
			Optional().
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}),
		field.JSON("reference_images", []map[string]any{}).
			Optional().
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}),
		field.JSON("result_images", []map[string]any{}).
			Optional().
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}),
		field.JSON("response", map[string]any{}).
			Optional().
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}),
		field.String("error").
			SchemaType(map[string]string{dialect.Postgres: "text"}).
			Default(""),
		field.JSON("usage", map[string]any{}).
			Optional().
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}),
		field.Float("cost").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,10)"}).
			Default(0),
		field.Int("duration_ms").
			Optional().
			Nillable(),
		field.JSON("metadata", map[string]any{}).
			Optional().
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}),
	}
}

func (PlaygroundImageTask) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).
			Ref("playground_image_tasks").
			Field("user_id").
			Required().
			Unique(),
		edge.From("api_key", APIKey.Type).
			Ref("playground_image_tasks").
			Field("api_key_id").
			Unique(),
	}
}

func (PlaygroundImageTask) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id", "created_at"),
		index.Fields("api_key_id"),
		index.Fields("status"),
		index.Fields("model"),
		index.Fields("deleted_at"),
	}
}
