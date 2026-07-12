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

// OidcClient 代表 sub2api 作为 OIDC Provider 时注册的第三方客户端 (RP)。
// 由 admin 手工录入；client_secret 永远不存明文，仅存 bcrypt hash。
type OidcClient struct {
	ent.Schema
}

func (OidcClient) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "oidc_clients"},
	}
}

func (OidcClient) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
	}
}

func (OidcClient) Fields() []ent.Field {
	return []ent.Field{
		// 业务主键，对外暴露的 client_id（如 "rp_<base32>"）
		field.String("client_id").
			MaxLen(64).
			NotEmpty().
			Unique(),
		// bcrypt hash，永不存明文 secret
		field.String("client_secret_hash").
			NotEmpty().
			SchemaType(map[string]string{dialect.Postgres: "text"}),
		field.String("client_name").
			MaxLen(100).
			NotEmpty(),
		// 严格相等匹配的 redirect_uri 列表
		field.JSON("redirect_uris", []string{}).
			Default([]string{}).
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}),
		// 该 client 允许申请的 scope 子集
		field.JSON("allowed_scopes", []string{}).
			Default([]string{}).
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}),
		// 固定 ["authorization_code","refresh_token"]，保留为字段方便日后扩展
		field.JSON("grant_types", []string{}).
			Default(func() []string { return []string{"authorization_code", "refresh_token"} }).
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}),
		field.Bool("consent_required").
			Default(true),
		field.Bool("enabled").
			Default(true),
	}
}

func (OidcClient) Indexes() []ent.Index {
	return []ent.Index{
		// client_id 已 Unique() 声明
		index.Fields("enabled"),
	}
}
