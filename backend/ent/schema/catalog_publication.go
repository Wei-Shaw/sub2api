package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
)

// CatalogPublication is the mutable pointer to the currently active immutable
// revision for a scope. Scope is the business key; the surrogate ID keeps the
// schema compatible with the repository's global Ent idtype configuration.
type CatalogPublication struct {
	ent.Schema
}

func (CatalogPublication) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "catalog_publications"}}
}

func (CatalogPublication) Fields() []ent.Field {
	return []ent.Field{
		field.String("scope").Unique(),
		field.Int64("active_revision_id"),
		field.Int64("epoch"),
		field.Time("updated_at").Default(time.Now),
	}
}
