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

// AuthIdentityChannel stores channel-scoped identifiers for a canonical identity.
type AuthIdentityChannel struct {
	ent.Schema
REDACTED

func (AuthIdentityChannel) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "auth_identity_channels"REDACTED,
REDACTED
REDACTED

func (AuthIdentityChannel) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{REDACTED,
REDACTED
REDACTED

func (AuthIdentityChannel) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("identity_id"),
		field.String("provider_type").
			MaxLen(20).
			NotEmpty().
			Validate(validateAuthProviderType),
		field.String("provider_key").
			NotEmpty().
			SchemaType(map[string]string{dialect.Postgres: "text"REDACTED),
		field.String("channel").
			MaxLen(20).
			NotEmpty(),
		field.String("channel_app_id").
			NotEmpty().
			SchemaType(map[string]string{dialect.Postgres: "text"REDACTED),
		field.String("channel_subject").
			NotEmpty().
			SchemaType(map[string]string{dialect.Postgres: "text"REDACTED),
		field.JSON("metadata", map[string]any{REDACTED).
			Default(func() map[string]any { return map[string]any{REDACTED REDACTED).
			SchemaType(map[string]string{dialect.Postgres: "jsonb"REDACTED),
REDACTED
REDACTED

func (AuthIdentityChannel) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("identity", AuthIdentity.Type).
			Ref("channels").
			Field("identity_id").
			Required().
			Unique(),
REDACTED
REDACTED

func (AuthIdentityChannel) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("provider_type", "provider_key", "channel", "channel_app_id", "channel_subject").Unique(),
		index.Fields("identity_id"),
REDACTED
REDACTED
