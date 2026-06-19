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

// SsoSession HttpOnly SSO 会话表，承载 sub2api_sso cookie。
// 仅供 /oidc/authorize 浏览器跳转识别登录态使用，与前端 localStorage JWT 完全独立。
type SsoSession struct {
	ent.Schema
}

func (SsoSession) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "sso_sessions"},
	}
}

func (SsoSession) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
	}
}

func (SsoSession) Fields() []ent.Field {
	return []ent.Field{
		// 32B base64url 随机值，cookie value 直接持有
		field.String("session_id").
			MaxLen(128).
			NotEmpty().
			Unique(),
		// 软引用 users.id；DB 层启用 cascade（在 SQL migration 中显式声明）
		field.Int64("user_id"),
		field.Time("issued_at").
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("last_seen_at").
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("expires_at").
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("revoked_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		// 用户 TOTP 是否已在本会话内验证（影响 ID Token 的 amr/acr）
		field.Time("totp_verified_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.String("user_agent").
			Default("").
			SchemaType(map[string]string{dialect.Postgres: "text"}),
		field.String("ip_address").
			Default("").
			SchemaType(map[string]string{dialect.Postgres: "text"}),
	}
}

func (SsoSession) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).
			Ref("sso_sessions").
			Field("user_id").
			Unique().
			Required(),
	}
}

func (SsoSession) Indexes() []ent.Index {
	return []ent.Index{
		// session_id Unique() 已声明
		index.Fields("user_id"),
		index.Fields("expires_at"),
	}
}
