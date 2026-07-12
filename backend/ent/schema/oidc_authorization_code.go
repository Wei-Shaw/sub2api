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

// OidcAuthorizationCode 一次性短生命周期授权码 (默认 10 分钟)。
// 绑定 client_id + redirect_uri + PKCE code_challenge + nonce。
type OidcAuthorizationCode struct {
	ent.Schema
}

func (OidcAuthorizationCode) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "oidc_authorization_codes"},
	}
}

func (OidcAuthorizationCode) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
	}
}

func (OidcAuthorizationCode) Fields() []ent.Field {
	return []ent.Field{
		// 32B base64url，一次性 token 值
		field.String("code").
			MaxLen(128).
			NotEmpty().
			Unique(),
		field.String("client_id").
			MaxLen(64).
			NotEmpty(),
		field.Int64("user_id"),
		field.String("redirect_uri").
			NotEmpty().
			SchemaType(map[string]string{dialect.Postgres: "text"}),
		field.JSON("scopes", []string{}).
			Default([]string{}).
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}),
		// PKCE
		field.String("code_challenge").
			MaxLen(128).
			NotEmpty(),
		field.String("code_challenge_method").
			MaxLen(10).
			Default("S256"),
		// 透传到 ID Token
		field.String("nonce").
			Default("").
			SchemaType(map[string]string{dialect.Postgres: "text"}),
		field.Time("expires_at").
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("consumed_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (OidcAuthorizationCode) Indexes() []ent.Index {
	return []ent.Index{
		// code 字段 Unique() 已声明
		index.Fields("expires_at"),
		index.Fields("user_id"),
		index.Fields("client_id"),
	}
}
