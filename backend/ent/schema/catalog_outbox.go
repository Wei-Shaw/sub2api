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

// CatalogOutbox is the durable publication notification stream. It is separate
// from scheduler_outbox so retention, cleanup, and cache namespaces cannot be
// accidentally coupled.
type CatalogOutbox struct {
	ent.Schema
}

func (CatalogOutbox) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "catalog_outbox"}}
}

func (CatalogOutbox) Fields() []ent.Field {
	return []ent.Field{
		field.String("event_type"),
		field.String("scope"),
		field.Int64("publication_epoch"),
		field.Int64("catalog_revision_id"),
		field.Int64("model_id").Optional().Nillable(),
		field.JSON("payload", map[string]any{}).
			Optional().
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}),
		field.String("dedup_key").Optional().Nillable(),
		field.Time("created_at").Default(time.Now),
	}
}

func (CatalogOutbox) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("scope", "publication_epoch", "id"),
		index.Fields("dedup_key").Unique(),
	}
}
