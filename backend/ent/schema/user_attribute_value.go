package schema

import (
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// UserAttributeValue holds a user's value for a specific attribute.
//
// This entity stores the actual values that users have for each attribute definition.
// Values are stored as strings and converted to the appropriate type by the application.
type UserAttributeValue struct {
	ent.Schema
}

func (UserAttributeValue) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "user_attribute_values"},
	}
}

func (UserAttributeValue) Mixin() []ent.Mixin {
	return []ent.Mixin{
		// Only use TimeMixin, no soft delete - values are hard deleted
		mixins.TimeMixin{},
	}
}

func (UserAttributeValue) Fields() []ent.Field {
	return []ent.Field{
		// user_id: References the user this value belongs to
		field.Int64("user_id"),

		// attribute_id: References the attribute definition
		field.Int64("attribute_id"),

		// value: The actual value stored as a string
		// For multi_select, this is a JSON array string
		field.Text("value").
			Default(""),
	}
}

func (UserAttributeValue) Edges() []ent.Edge {
	return []ent.Edge{
		// user: The user who owns this attribute value
		edge.From("user", User.Type).
			Ref("attribute_values").
			Field("user_id").
			Required().
			Unique(),

		// definition: The attribute definition this value is for
		edge.From("definition", UserAttributeDefinition.Type).
			Ref("values").
			Field("attribute_id").
			Required().
			Unique(),
	}
}

func (UserAttributeValue) Indexes() []ent.Index {
	return []ent.Index{
		// Unique index on (user_id, attribute_id)
		index.Fields("user_id", "attribute_id").Unique(),
		index.Fields("attribute_id"),
	}
}
