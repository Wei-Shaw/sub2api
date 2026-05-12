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

type CacheInvalidationOutbox struct {
	ent.Schema
}

func (CacheInvalidationOutbox) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "cache_invalidation_outbox"},
	}
}

func (CacheInvalidationOutbox) Fields() []ent.Field {
	return []ent.Field{
		field.String("event_type").
			MaxLen(64).
			NotEmpty(),
		field.String("aggregate_type").
			MaxLen(64).
			NotEmpty(),
		field.Int64("aggregate_id").
			Optional().
			Nillable(),
		field.String("reason").
			MaxLen(128).
			NotEmpty(),
		field.JSON("cache_types", []string{}).
			SchemaType(map[string]string{dialect.Postgres: "text[]"}),
		field.JSON("payload", map[string]any{}).
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}),
		field.String("status").
			MaxLen(20).
			Default("pending"),
		field.Int("attempts").
			Default(0),
		field.Int("max_attempts").
			Default(12),
		field.Time("next_attempt_at").
			Default(time.Now).
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("locked_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.String("locked_by").
			MaxLen(128).
			Optional().
			Nillable(),
		field.Time("processed_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Text("last_error").
			Optional().
			Nillable(),
		field.String("idempotency_key").
			MaxLen(200).
			Optional().
			Nillable(),
		field.Time("created_at").
			Immutable().
			Default(time.Now).
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now).
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (CacheInvalidationOutbox) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("status", "next_attempt_at", "id").
			Annotations(entsql.IndexWhere("status IN ('pending', 'failed')")),
		index.Fields("locked_at").
			Annotations(entsql.IndexWhere("status = 'processing'")),
		index.Fields("created_at"),
		index.Fields("idempotency_key").
			Unique().
			Annotations(entsql.IndexWhere("idempotency_key IS NOT NULL")),
	}
}
