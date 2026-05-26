// Package schema 定义 Ent ORM 的数据库 schema。
package schema

import (
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
)

// TLSFingerprintProfile 定义 TLS 指纹配置模板的 schema。
//
// TLS 指纹模板用于模拟特定客户端（如 Claude Code / Node.js）的 TLS 握手特征。
// 每个模板包含完整的 ClientHello 参数：加密套件、曲线、扩展等。
// 通过 Account.Extra.tls_fingerprint_profile_id 绑定到具体账号。
type TLSFingerprintProfile struct {
	ent.Schema
}

// Annotations 返回 schema 的注解配置。
func (TLSFingerprintProfile) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "tls_fingerprint_profiles"},
	}
}

// Mixin 返回该 schema 使用的混入组件。
func (TLSFingerprintProfile) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
	}
}

// Fields 定义 TLS 指纹模板实体的所有字段。
func (TLSFingerprintProfile) Fields() []ent.Field {
	return []ent.Field{
		// name: 模板名称，唯一标识
		field.String("name").
			MaxLen(100).
			NotEmpty().
			Unique(),

		// description: 模板描述
		field.Text("description").
			Optional().
			Nillable(),

		// enable_grease: 是否启用 GREASE 扩展（Chrome 使用，Node.js 不使用）
		field.Bool("enable_grease").
			Default(false),

		// cipher_suites: TLS 加密套件列表（顺序敏感，影响 JA3）
		field.JSON("cipher_suites", []uint16{}).
			Optional().
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}),

		// curves: 椭圆曲线/支持的组列表
		field.JSON("curves", []uint16{}).
			Optional().
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}),

		// point_formats: EC 点格式列表
		field.JSON("point_formats", []uint16{}).
			Optional().
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}),

		// signature_algorithms: 签名算法列表
		field.JSON("signature_algorithms", []uint16{}).
			Optional().
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}),

		// alpn_protocols: ALPN 协议列表（如 ["http/1.1"]）
		field.JSON("alpn_protocols", []string{}).
			Optional().
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}),

		// supported_versions: 支持的 TLS 版本列表（如 [0x0304, 0x0303]）
		field.JSON("supported_versions", []uint16{}).
			Optional().
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}),

		// key_share_groups: Key Share 中发送的曲线组（如 [29] 即 X25519）
		field.JSON("key_share_groups", []uint16{}).
			Optional().
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}),

		// psk_modes: PSK 密钥交换模式（如 [1] 即 psk_dhe_ke）
		field.JSON("psk_modes", []uint16{}).
			Optional().
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}),

		// extensions: TLS 扩展类型 ID 列表，按发送顺序排列
		field.JSON("extensions", []uint16{}).
			Optional().
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}),
	}
}
