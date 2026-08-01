package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
)

// CatalogRevision is an immutable candidate snapshot.
type CatalogRevision struct {
	ent.Schema
}

func (CatalogRevision) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "catalog_revisions"}}
}

func (CatalogRevision) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("revision").Unique(),
		field.Int64("sync_run_id"),
		field.String("normalized_hash").Unique(),
		field.String("normalizer_version"),
		field.String("state"),
		field.Int("model_count"),
		field.Time("created_at").Default(time.Now),
		field.Time("validated_at").Optional().Nillable(),
		field.Time("published_at").Optional().Nillable(),
	}
}
