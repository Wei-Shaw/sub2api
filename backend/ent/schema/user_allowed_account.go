package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// UserAllowedAccount holds the edge schema definition for usage-viewer account access.
type UserAllowedAccount struct {
	ent.Schema
}

func (UserAllowedAccount) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "user_allowed_accounts"},
		field.ID("user_id", "account_id"),
	}
}

func (UserAllowedAccount) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("user_id"),
		field.Int64("account_id"),
		field.Time("created_at").
			Immutable().
			Default(time.Now).
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (UserAllowedAccount) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("user", User.Type).
			Unique().
			Required().
			Field("user_id"),
		edge.To("account", Account.Type).
			Unique().
			Required().
			Field("account_id"),
	}
}

func (UserAllowedAccount) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("account_id"),
	}
}
