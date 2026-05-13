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

// ChatMessage holds the schema definition for persisted chat messages.
type ChatMessage struct {
	ent.Schema
}

func (ChatMessage) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "chat_messages"},
	}
}

func (ChatMessage) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
	}
}

func (ChatMessage) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("session_id"),
		field.Int64("user_id"),
		field.String("role").
			MaxLen(20).
			NotEmpty(),
		field.String("content").
			SchemaType(map[string]string{dialect.Postgres: "text"}).
			Default(""),
		field.String("status").
			MaxLen(20).
			Default("completed"),
		field.String("model").
			MaxLen(100).
			Optional().
			Nillable(),
		field.Int("duration_ms").
			Optional().
			Nillable(),
		field.Int64("usage_log_id").
			Optional().
			Nillable(),
		field.Float("actual_cost").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,10)"}),
		field.String("error_message").
			SchemaType(map[string]string{dialect.Postgres: "text"}).
			Optional().
			Nillable(),
	}
}

func (ChatMessage) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("session", ChatSession.Type).
			Ref("messages").
			Field("session_id").
			Required().
			Unique(),
		edge.From("user", User.Type).
			Ref("chat_messages").
			Field("user_id").
			Required().
			Unique(),
		edge.From("usage_log", UsageLog.Type).
			Ref("chat_messages").
			Field("usage_log_id").
			Unique(),
	}
}

func (ChatMessage) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("session_id", "created_at"),
		index.Fields("user_id", "created_at"),
		index.Fields("usage_log_id"),
		index.Fields("status"),
	}
}
