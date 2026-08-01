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

// CatalogLifecycleAudit is append-only business evidence for policy changes.
type CatalogLifecycleAudit struct {
	ent.Schema
}

func (CatalogLifecycleAudit) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "catalog_lifecycle_audits"}}
}

func (CatalogLifecycleAudit) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("model_id").Optional().Nillable(),
		field.Int64("catalog_revision_id").Optional().Nillable(),
		field.String("action"),
		field.String("actor_type"),
		field.Int64("actor_user_id").Optional().Nillable(),
		field.String("reason").Optional().Nillable(),
		field.JSON("before_state", map[string]any{}).
			Optional().
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}),
		field.JSON("after_state", map[string]any{}).
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}),
		field.String("request_id").Optional().Nillable(),
		field.String("correlation_id").Optional().Nillable(),
		field.Time("created_at").Default(time.Now),
	}
}

func (CatalogLifecycleAudit) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("model_id", "created_at"),
		index.Fields("catalog_revision_id", "created_at"),
	}
}
