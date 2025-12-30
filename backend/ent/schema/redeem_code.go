package schema

import (
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// RedeemCode holds the schema definition for the RedeemCode entity.
type RedeemCode struct {
	ent.Schema
REDACTED

func (RedeemCode) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "redeem_codes"REDACTED,
REDACTED
REDACTED

func (RedeemCode) Fields() []ent.Field {
	return []ent.Field{
		field.String("code").
			MaxLen(32).
			NotEmpty().
			Unique(),
		field.String("type").
			MaxLen(20).
			Default(service.RedeemTypeBalance),
		field.Float("value").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"REDACTED).
			Default(0),
		field.String("status").
			MaxLen(20).
			Default(service.StatusUnused),
		field.Int64("used_by").
			Optional().
			Nillable(),
		field.Time("used_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"REDACTED),
		field.String("notes").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "text"REDACTED),
		field.Time("created_at").
			Immutable().
			Default(time.Now).
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"REDACTED),
		field.Int64("group_id").
			Optional().
			Nillable(),
		field.Int("validity_days").
			Default(30),
REDACTED
REDACTED

func (RedeemCode) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).
			Ref("redeem_codes").
			Field("used_by").
			Unique(),
		edge.From("group", Group.Type).
			Ref("redeem_codes").
			Field("group_id").
			Unique(),
REDACTED
REDACTED

func (RedeemCode) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("code").Unique(),
		index.Fields("status"),
		index.Fields("used_by"),
		index.Fields("group_id"),
REDACTED
REDACTED
