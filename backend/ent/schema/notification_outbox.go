package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type NotificationOutbox struct{ ent.Schema }

func (NotificationOutbox) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "notification_outbox"}}
}

func (NotificationOutbox) Fields() []ent.Field {
	return []ent.Field{
		field.String("dedup_key").MaxLen(255).Immutable().Unique(),
		field.String("event").MaxLen(128).Immutable(),
		field.String("recipient").MaxLen(255).Immutable(),
		field.String("locale").MaxLen(16).Immutable().Default("en-US"),
		field.JSON("variables", map[string]string{}).Optional().Immutable(),
		field.String("status").MaxLen(16).Default("pending"),
		field.Int("attempt_count").Default(0),
		field.Time("next_attempt_at").Default(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("claimed_at").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.String("claimed_by_worker_id").MaxLen(64).Optional().Nillable(),
		field.Time("delivered_at").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.String("last_error").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "text"}),
		field.Time("created_at").Immutable().Default(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (NotificationOutbox) Indexes() []ent.Index {
	return []ent.Index{index.Fields("status", "next_attempt_at", "id")}
}
