package schema

import (
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// InnerAPIApp 代表一个接入内部 API RPC 的服务 app。
// 鉴权采用无状态 token：token = AES-256-GCM(本地密钥, payload{app_id, version})，
// DB 不存任何密文/hash；本表仅做接入方注册（app_id / 名称 / 启停）与审计。
type InnerAPIApp struct {
	ent.Schema
}

func (InnerAPIApp) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "inner_api_apps"},
	}
}

func (InnerAPIApp) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
	}
}

func (InnerAPIApp) Fields() []ent.Field {
	return []ent.Field{
		// 对外暴露的业务主键，如 "iapp_<base32>"
		field.String("app_id").
			MaxLen(64).
			NotEmpty().
			Unique(),
		field.String("app_name").
			MaxLen(100).
			NotEmpty(),
		field.Bool("enabled").
			Default(true),
		// token 版本：刷新 token 时 +1，使旧 token（携带旧版本）失效。
		field.Int("token_version").
			Default(1),
		// 方法级授权；只允许 service 层定义的四种权限值。
		field.JSON("permissions", []string{}).
			Default([]string{}),
	}
}

func (InnerAPIApp) Indexes() []ent.Index {
	return []ent.Index{
		// app_id 已 Unique() 声明
		index.Fields("enabled"),
	}
}
