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

// CatalogModelRevision contains one immutable model definition and normalized
// pricing payload for one catalog revision.
type CatalogModelRevision struct {
	ent.Schema
}

func (CatalogModelRevision) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "catalog_model_revisions"}}
}

func (CatalogModelRevision) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("catalog_revision_id"),
		field.Int64("model_id"),
		field.String("source_state"),
		field.String("provider"),
		field.String("platform"),
		field.String("mode"),
		field.JSON("capabilities", map[string]any{}).
			Default(map[string]any{}).
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}),
		field.Int64("context_window").Optional().Nillable(),
		field.Int64("max_output_tokens").Optional().Nillable(),
		field.Int("pricing_schema_version"),
		field.JSON("pricing_json", map[string]any{}).
			Optional().
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}),
		field.Bool("pricing_valid").Default(false),
		field.String("operator_state").Default("enabled"),
		field.String("operator_reason").Optional().Nillable(),
		field.Int64("operator_version").Default(1),
		field.String("pricing_source").Optional().Nillable(),
		field.JSON("source_metadata", map[string]any{}).
			Default(map[string]any{}).
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}),
		field.String("source_hash"),
		field.Time("created_at").Default(time.Now),
	}
}

func (CatalogModelRevision) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("catalog_revision_id", "model_id").Unique(),
		index.Fields("catalog_revision_id"),
		index.Fields("model_id"),
	}
}
