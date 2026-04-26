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

// UserDevice holds the schema definition for the UserDevice entity.
type UserDevice struct {
	ent.Schema
}

func (UserDevice) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "user_devices"},
	}
}

func (UserDevice) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("user_id"),
		field.String("device_hash").MaxLen(64).NotEmpty(),
		field.Int("fingerprint_version").Default(1),
		field.String("install_id").Optional().Nillable().MaxLen(128),
		field.String("platform").MaxLen(32).NotEmpty(),
		field.String("arch").MaxLen(16).NotEmpty(),
		field.String("app_version").Optional().Nillable().MaxLen(32),
		field.Int64("claim_redeem_code_id").Optional().Nillable(),
		field.Int64("login_redeem_code_id"),
		field.String("status").MaxLen(20).Default("active"),
		field.Time("first_claimed_at").Default(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("last_claimed_at").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("last_login_at").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("created_at").Immutable().Default(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (UserDevice) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).
			Ref("devices").
			Field("user_id").
			Required().
			Unique(),
		edge.From("claim_redeem_code", RedeemCode.Type).
			Ref("claimed_devices").
			Field("claim_redeem_code_id").
			Unique(),
		edge.From("login_redeem_code", RedeemCode.Type).
			Ref("login_devices").
			Field("login_redeem_code_id").
			Required().
			Unique(),
	}
}

func (UserDevice) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("device_hash").Unique(),
		index.Fields("claim_redeem_code_id").Unique(),
		index.Fields("login_redeem_code_id").Unique(),
		index.Fields("user_id"),
		index.Fields("status"),
	}
}
