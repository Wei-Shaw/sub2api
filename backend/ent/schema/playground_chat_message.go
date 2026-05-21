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

// PlaygroundChatMessage holds the schema definition for a Playground chat message.
type PlaygroundChatMessage struct {
	ent.Schema
}

func (PlaygroundChatMessage) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "playground_chat_messages"},
	}
}

func (PlaygroundChatMessage) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
	}
}

func (PlaygroundChatMessage) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("session_id"),
		field.Int64("user_id"),
		field.Int64("api_key_id").
			Optional().
			Nillable(),
		field.String("role").
			MaxLen(20),
		field.String("model").
			MaxLen(100).
			Default(""),
		field.String("content").
			SchemaType(map[string]string{dialect.Postgres: "text"}).
			Default(""),
		field.JSON("content_json", map[string]any{}).
			Optional().
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}),
		field.JSON("images", []map[string]any{}).
			Optional().
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}),
		field.JSON("usage", map[string]any{}).
			Optional().
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}),
		field.String("status").
			MaxLen(20).
			Default("success"),
		field.String("error").
			SchemaType(map[string]string{dialect.Postgres: "text"}).
			Default(""),
		field.Int("duration_ms").
			Optional().
			Nillable(),
		field.JSON("metadata", map[string]any{}).
			Optional().
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}),
	}
}

func (PlaygroundChatMessage) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("session", PlaygroundChatSession.Type).
			Ref("messages").
			Field("session_id").
			Required().
			Unique(),
		edge.From("user", User.Type).
			Ref("playground_chat_messages").
			Field("user_id").
			Required().
			Unique(),
		edge.From("api_key", APIKey.Type).
			Ref("playground_chat_messages").
			Field("api_key_id").
			Unique(),
	}
}

func (PlaygroundChatMessage) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("session_id", "created_at"),
		index.Fields("user_id", "created_at"),
		index.Fields("api_key_id"),
		index.Fields("role"),
		index.Fields("status"),
	}
}
