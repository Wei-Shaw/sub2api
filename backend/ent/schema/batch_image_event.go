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

// BatchImageEvent records append-only operational events for batch image jobs.
type BatchImageEvent struct {
	ent.Schema
}

func (BatchImageEvent) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "batch_image_events"},
	}
}

func (BatchImageEvent) Fields() []ent.Field {
	return []ent.Field{
		field.String("job_id").MaxLen(64),
		field.String("event_type").MaxLen(64),
		field.JSON("payload", map[string]any{}).
			Optional().
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}),
		field.String("event_hash").Optional().Nillable().MaxLen(128),
		field.Time("created_at").Immutable().Default(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (BatchImageEvent) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("job_id", "created_at"),
		index.Fields("event_type"),
		index.Fields("job_id", "event_hash").Unique().Annotations(entsql.IndexWhere("event_hash IS NOT NULL AND event_hash <> ''")),
	}
}
