package schema

import (
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Proxy holds the schema definition for the Proxy entity.
type Proxy struct {
	ent.Schema
REDACTED

func (Proxy) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "proxies"REDACTED,
REDACTED
REDACTED

func (Proxy) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{REDACTED,
		mixins.SoftDeleteMixin{REDACTED,
REDACTED
REDACTED

func (Proxy) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").
			MaxLen(100).
			NotEmpty(),
		field.String("protocol").
			MaxLen(20).
			NotEmpty(),
		field.String("host").
			MaxLen(255).
			NotEmpty(),
		field.Int("port"),
		field.String("username").
			MaxLen(100).
			Optional().
			Nillable(),
		field.String("password").
			MaxLen(100).
			Optional().
			Nillable(),
		field.String("status").
			MaxLen(20).
			Default("active"),
REDACTED
REDACTED

func (Proxy) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("status"),
		index.Fields("deleted_at"),
REDACTED
REDACTED
