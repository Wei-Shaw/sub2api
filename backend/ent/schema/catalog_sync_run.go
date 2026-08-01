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

// CatalogSyncRun records every import attempt, including rejected/failed runs.
type CatalogSyncRun struct {
	ent.Schema
}

func (CatalogSyncRun) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "catalog_sync_runs"}}
}

func (CatalogSyncRun) Fields() []ent.Field {
	return []ent.Field{
		field.String("source_set"),
		field.String("trigger"),
		field.Int64("actor_user_id").Optional().Nillable(),
		field.String("upstream_version").Optional().Nillable(),
		field.String("upstream_etag").Optional().Nillable(),
		field.String("upstream_hash").Optional().Nillable(),
		field.String("normalized_hash").Optional().Nillable(),
		field.String("normalizer_version"),
		field.String("status"),
		field.Int("source_count").Default(0),
		field.Int("normalized_count").Default(0),
		field.Int("added_count").Default(0),
		field.Int("changed_count").Default(0),
		field.Int("missing_count").Default(0),
		field.Int("invalid_count").Default(0),
		field.JSON("validation_errors", []any{}).
			Default([]any{}).
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}),
		field.Time("started_at").Default(time.Now),
		field.Time("completed_at").Optional().Nillable(),
	}
}

func (CatalogSyncRun) Indexes() []ent.Index {
	return []ent.Index{index.Fields("status", "started_at")}
}
