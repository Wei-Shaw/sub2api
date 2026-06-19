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

// OidcRefreshToken OIDC refresh token (opaque, 非 JWT)。
// 同一次授权链上的所有 refresh 共享 family_id；任意一个被复用 → 整个 family 立即吊销。
type OidcRefreshToken struct {
	ent.Schema
}

func (OidcRefreshToken) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "oidc_refresh_tokens"},
	}
}

func (OidcRefreshToken) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
	}
}

func (OidcRefreshToken) Fields() []ent.Field {
	return []ent.Field{
		// 32B base64url，opaque token 值；查询入口
		field.String("token").
			MaxLen(128).
			NotEmpty().
			Unique(),
		// 同授权链上的所有 refresh 共享同一 family_id
		field.String("family_id").
			MaxLen(64).
			NotEmpty(),
		field.String("client_id").
			MaxLen(64).
			NotEmpty(),
		field.Int64("user_id"),
		field.JSON("scopes", []string{}).
			Default([]string{}).
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}),
		field.Time("expires_at").
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("revoked_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		// 上一代的 token hash（sha256 hex），便于审计
		field.String("parent_token_hash").
			Default("").
			SchemaType(map[string]string{dialect.Postgres: "text"}),
	}
}

func (OidcRefreshToken) Indexes() []ent.Index {
	return []ent.Index{
		// token Unique() 已声明
		index.Fields("family_id"),
		index.Fields("user_id"),
		index.Fields("client_id"),
		index.Fields("expires_at"),
	}
}
