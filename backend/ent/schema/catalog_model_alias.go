package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// CatalogModelAlias stores explicit deterministic aliases; runtime must not
// invent fuzzy aliases from unknown request strings.
type CatalogModelAlias struct {
	ent.Schema
}

func (CatalogModelAlias) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "catalog_model_aliases"}}
}

func (CatalogModelAlias) Fields() []ent.Field {
	return []ent.Field{
		field.String("alias_normalized"),
		field.String("platform_scope").Default("*"),
		field.Int64("model_id"),
		field.String("source"),
		field.String("state"),
		field.Int64("introduced_revision_id").Optional().Nillable(),
		field.Int64("retired_revision_id").Optional().Nillable(),
		field.Time("created_at").Default(time.Now),
		field.Time("updated_at").Default(time.Now),
	}
}

func (CatalogModelAlias) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("platform_scope", "alias_normalized").Unique(),
		index.Fields("model_id"),
	}
}
