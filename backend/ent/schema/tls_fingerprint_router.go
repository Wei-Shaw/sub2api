// Package schema 定义 Ent ORM 的数据库 schema。
package schema

import (
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"
	"github.com/Wei-Shaw/sub2api/internal/model"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// TLSFingerprintRouter 定义 TLS 指纹路由器 schema。
//
// TLS 路由器按入站 User-Agent 选择 TLS 指纹模板，账号可在固定模板之上绑定一个
// 路由器，实现不同客户端运行时使用不同 TLS ClientHello。
type TLSFingerprintRouter struct {
	ent.Schema
}

// Annotations 返回 schema 的注解配置。
func (TLSFingerprintRouter) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "tls_fingerprint_routers"},
	}
}

// Mixin 返回该 schema 使用的混入组件。
func (TLSFingerprintRouter) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
	}
}

// Fields 定义 TLS 路由器实体的所有字段。
func (TLSFingerprintRouter) Fields() []ent.Field {
	return []ent.Field{
		// name: 路由器名称，唯一标识。
		field.String("name").
			MaxLen(100).
			NotEmpty().
			Unique(),

		// description: 路由器描述。
		field.Text("description").
			Optional().
			Nillable(),

		// enabled: 是否启用该路由器。
		field.Bool("enabled").
			Default(true),

		// chatgpt_oauth_token_user_agent: ChatGPT OAuth token 请求使用的 UA，空值使用系统兜底。
		field.String("chatgpt_oauth_token_user_agent").
			MaxLen(512).
			Default(""),

		// chatgpt_oauth_token_tls_fingerprint_profile_id: ChatGPT OAuth token 请求使用的 TLS 模板。
		// nil 表示不启用 token 专用 TLS 模板；0 表示内置默认模板；-1 表示随机模板；正数表示指定模板。
		field.Int64("chatgpt_oauth_token_tls_fingerprint_profile_id").
			Optional().
			Nillable(),

		// codex_invite_reset_user_agent: Codex 邀请重置请求使用的 UA，空值使用 Desktop 兜底。
		field.String("codex_invite_reset_user_agent").
			MaxLen(512).
			Default(""),

		// codex_invite_reset_tls_fingerprint_profile_id: Codex 邀请重置请求使用的 TLS 模板。
		// nil 表示沿用账号 TLS 模板；0 表示内置默认模板；-1 表示随机模板；正数表示指定模板。
		field.Int64("codex_invite_reset_tls_fingerprint_profile_id").
			Optional().
			Nillable(),

		// rules: 按顺序匹配的 UA 规则列表，命中第一条后返回对应 TLS 模板。
		field.JSON("rules", []model.TLSFingerprintRouterRule{}).
			Optional().
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}),
	}
}

// Indexes 定义数据库索引，优化查询性能。
func (TLSFingerprintRouter) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("enabled"),
	}
}
