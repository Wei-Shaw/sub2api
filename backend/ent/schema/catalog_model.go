// Package schema 定义 Ent ORM 的数据库 schema。
package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// CatalogModel stores stable model identity and operator-owned lifecycle policy.
// Source refreshes must never overwrite disabled or retired policy implicitly.
type CatalogModel struct {
	ent.Schema
}

func (CatalogModel) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "catalog_models"}}
}

func (CatalogModel) Fields() []ent.Field {
	return []ent.Field{
		field.String("canonical_key"),
		field.String("canonical_key_normalized").Unique(),
		field.String("operator_state").Default("enabled"),
		field.String("operator_reason").Optional().Nillable(),
		field.Int64("replacement_model_id").Optional().Nillable(),
		field.Int64("operator_version").Default(1),
		field.Time("first_seen_at").Default(time.Now),
		field.Time("last_operator_change_at").Optional().Nillable(),
		field.Time("retired_at").Optional().Nillable(),
		field.Time("created_at").Default(time.Now),
		field.Time("updated_at").Default(time.Now),
	}
}

func (CatalogModel) Indexes() []ent.Index {
	return []ent.Index{index.Fields("operator_state")}
}
