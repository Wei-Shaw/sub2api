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

// OidcAccessToken OIDC 访问令牌 (opaque, 非 JWT)，仅用于 /oidc/userinfo 的 Bearer 鉴权。
// 与 sub2api 自身 HS256 access token 完全隔离 (后者继续走 cfg.JWT.Secret 签名)。
type OidcAccessToken struct {
	ent.Schema
}

func (OidcAccessToken) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "oidc_access_tokens"},
	}
}

func (OidcAccessToken) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
	}
}

func (OidcAccessToken) Fields() []ent.Field {
	return []ent.Field{
		// 32B base64url
		field.String("token").
			MaxLen(128).
			NotEmpty().
			Unique(),
		field.String("client_id").
			MaxLen(64).
			NotEmpty(),
		field.Int64("user_id"),
		field.JSON("scopes", []string{}).
			Default([]string{}).
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}),
		// 关联 refresh family，便于 family rotation 时一并清理
		field.String("refresh_family_id").
			Default("").
			MaxLen(64),
		field.Time("expires_at").
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("revoked_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (OidcAccessToken) Indexes() []ent.Index {
	return []ent.Index{
		// token Unique() 已声明
		index.Fields("user_id"),
		index.Fields("client_id"),
		index.Fields("expires_at"),
		index.Fields("refresh_family_id"),
	}
}
