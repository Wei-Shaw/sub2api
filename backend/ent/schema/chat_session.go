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

// ChatSession holds the schema definition for persisted user chat sessions.
type ChatSession struct {
	ent.Schema
}

func (ChatSession) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "chat_sessions"},
	}
}

func (ChatSession) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
		mixins.SoftDeleteMixin{},
	}
}

func (ChatSession) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("user_id"),
		field.Int64("api_key_id"),
		field.String("title").
			MaxLen(160).
			NotEmpty(),
		field.String("model").
			MaxLen(100).
			NotEmpty(),
		field.String("status").
			MaxLen(20).
			Default("active"),
		field.Time("expires_at").
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}).
			Default(func() time.Time {
				return time.Now().Add(30 * 24 * time.Hour)
			}),
	}
}

func (ChatSession) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).
			Ref("chat_sessions").
			Field("user_id").
			Required().
			Unique(),
		edge.From("api_key", APIKey.Type).
			Ref("chat_sessions").
			Field("api_key_id").
			Required().
			Unique(),
		edge.To("messages", ChatMessage.Type),
	}
}

func (ChatSession) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id", "updated_at"),
		index.Fields("user_id", "expires_at"),
		index.Fields("user_id", "deleted_at"),
		index.Fields("api_key_id"),
		index.Fields("status"),
	}
}
