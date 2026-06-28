package schema

import (
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// BillingApp 代表一个接入余额 RPC 的「扣费 app」。
// 鉴权采用无状态 token：app 的 secret = AES-256-GCM(本地密钥, payload{app_id})，
// DB 不存任何密文/hash；本表仅做接入方注册（app_id / 名称 / 启停）与审计。
type BillingApp struct {
	ent.Schema
}

func (BillingApp) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "billing_apps"},
	}
}

func (BillingApp) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
	}
}

func (BillingApp) Fields() []ent.Field {
	return []ent.Field{
		// 对外暴露的业务主键，如 "bapp_<base32>"
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
	}
}

func (BillingApp) Indexes() []ent.Index {
	return []ent.Index{
		// app_id 已 Unique() 声明
		index.Fields("enabled"),
	}
}
