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

// PlaygroundChatSession holds the schema definition for a user's Playground chat session.
type PlaygroundChatSession struct {
	ent.Schema
}

func (PlaygroundChatSession) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "playground_chat_sessions"},
	}
}

func (PlaygroundChatSession) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
		mixins.SoftDeleteMixin{},
	}
}

func (PlaygroundChatSession) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("user_id"),
		field.String("title").
			MaxLen(200).
			Default("新会话"),
		field.String("model").
			MaxLen(100).
			Default(""),
		field.Int64("api_key_id").
			Optional().
			Nillable(),
		field.String("system_prompt").
			SchemaType(map[string]string{dialect.Postgres: "text"}).
			Default(""),
		field.Bool("use_context").
			Default(true),
		field.JSON("metadata", map[string]any{}).
			Optional().
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}),
		field.Time("last_message_at").
			Default(time.Now).
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (PlaygroundChatSession) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).
			Ref("playground_chat_sessions").
			Field("user_id").
			Required().
			Unique(),
		edge.From("api_key", APIKey.Type).
			Ref("playground_chat_sessions").
			Field("api_key_id").
			Unique(),
		edge.To("messages", PlaygroundChatMessage.Type).
			Annotations(entsql.OnDelete(entsql.Cascade)),
	}
}

func (PlaygroundChatSession) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id", "last_message_at"),
		index.Fields("user_id", "updated_at"),
		index.Fields("api_key_id"),
		index.Fields("deleted_at"),
	}
}
