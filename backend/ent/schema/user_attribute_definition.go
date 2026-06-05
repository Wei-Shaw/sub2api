package schema

import (
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// UserAttributeDefinition holds the schema definition for custom user attributes.
//
// This entity defines the metadata for user attributes, such as:
//   - Attribute key (unique identifier like "company_name")
//   - Display name shown in forms
//   - Field type (text, number, select, etc.)
//   - Validation rules
//   - Whether the field is required or enabled
type UserAttributeDefinition struct {
	ent.Schema
}

func (UserAttributeDefinition) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "user_attribute_definitions"},
	}
}

func (UserAttributeDefinition) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
		mixins.SoftDeleteMixin{},
	}
}

func (UserAttributeDefinition) Fields() []ent.Field {
	return []ent.Field{
		// key: Unique identifier for the attribute (e.g., "company_name")
		// Used for programmatic reference
		field.String("key").
			MaxLen(100).
			NotEmpty(),

		// name: Display name shown in forms (e.g., "Company Name")
		field.String("name").
			MaxLen(255).
			NotEmpty(),

		// description: Optional description/help text for the attribute
		field.String("description").
			SchemaType(map[string]string{dialect.Postgres: "text"}).
			Default(""),

		// type: Attribute type - text, textarea, number, email, url, date, select, multi_select
		field.String("type").
			MaxLen(20).
			NotEmpty(),

		// options: Select options for select/multi_select types (stored as JSONB)
		// Format: [{"value": "xxx", "label": "XXX"}, ...]
		field.JSON("options", []map[string]any{}).
			Default([]map[string]any{}).
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}),

		// required: Whether this attribute is required when editing a user
		field.Bool("required").
			Default(false),

		// validation: Validation rules for the attribute value (stored as JSONB)
		// Format: {"min_length": 1, "max_length": 100, "min": 0, "max": 100, "pattern": "^[a-z]+$", "message": "..."}
		field.JSON("validation", map[string]any{}).
			Default(map[string]any{}).
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}),

		// placeholder: Placeholder text shown in input fields
		field.String("placeholder").
			MaxLen(255).
			Default(""),

		// display_order: Order in which attributes are displayed (lower = first)
		field.Int("display_order").
			Default(0),

		// enabled: Whether this attribute is active and shown in forms
		field.Bool("enabled").
			Default(true),
	}
}

func (UserAttributeDefinition) Edges() []ent.Edge {
	return []ent.Edge{
		// values: All user values for this attribute definition
		edge.To("values", UserAttributeValue.Type),
	}
}

func (UserAttributeDefinition) Indexes() []ent.Index {
	return []ent.Index{
		// Partial unique index on key (WHERE deleted_at IS NULL) via migration
		index.Fields("key"),
		index.Fields("enabled"),
		index.Fields("display_order"),
		index.Fields("deleted_at"),
	}
}
