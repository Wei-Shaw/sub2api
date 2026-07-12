package schema

import (
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// OidcConsent 记录 (user_id, client_id) 维度的同意 scope 集合。
// 后续 /oidc/authorize 命中时，若 granted_scopes ⊇ 本次请求 scopes 则跳过 consent 页。
type OidcConsent struct {
	ent.Schema
}

func (OidcConsent) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "oidc_consents"},
	}
}

func (OidcConsent) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
	}
}

func (OidcConsent) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("user_id"),
		field.String("client_id").
			MaxLen(64).
			NotEmpty(),
		field.JSON("granted_scopes", []string{}).
			Default([]string{}).
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}),
		field.Time("granted_at").
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("last_used_at").
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (OidcConsent) Indexes() []ent.Index {
	return []ent.Index{
		// (user_id, client_id) 唯一：每个用户对每个 client 一条 consent 记录
		index.Fields("user_id", "client_id").Unique(),
		index.Fields("client_id"),
	}
}
