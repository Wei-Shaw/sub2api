// Package schema defines the ent schemas for the channel-management plugin.
//
// These schemas describe the existing database tables -- the plugin uses
// declarative SQL migrations (plugins/channel-management/migrations/) for
// DDL, NOT ent's Schema.Create auto-migration. The schemas here are
// strictly for ent's query builder / codegen.
package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
)

// Channel maps the "channels" table owned by the channel-management plugin.
//
// Only columns that the plugin directly reads/writes are declared.
// Edges (channel_groups, channel_model_pricing, etc.) are deliberately
// omitted -- the plugin manages associations via hand-written SQL and will
// only add ent edges during a full migration.
type Channel struct {
	ent.Schema
}

// Annotations pins this schema to the existing "channels" table.
func (Channel) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "channels"},
	}
}

// Fields mirrors the channels table columns.
func (Channel) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id"),

		field.String("name").
			MaxLen(100).
			Unique(),

		field.String("description").
			Default(""),

		field.String("status").
			MaxLen(20).
			Default("active"),

		field.JSON("model_mapping", map[string]map[string]string{}).
			Optional().
			Default(map[string]map[string]string{}),

		field.String("billing_model_source").
			MaxLen(20).
			Default("channel_mapped"),

		field.Bool("restrict_models").
			Default(false),

		field.String("features").
			Default(""),

		field.JSON("features_config", map[string]any{}).
			Default(map[string]any{}),

		field.Bool("apply_pricing_to_account_stats").
			Default(false),

		field.Time("created_at").
			Default(time.Now).
			Immutable().
			SchemaType(map[string]string{
				"postgres": "timestamptz",
			}),

		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now).
			SchemaType(map[string]string{
				"postgres": "timestamptz",
			}),
	}
}
