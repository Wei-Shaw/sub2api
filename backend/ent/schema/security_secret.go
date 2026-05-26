package schema

import (
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
)

// SecuritySecret 存储系统级安全密钥（如 JWT 签名密钥、TOTP 加密密钥）。
type SecuritySecret struct {
	ent.Schema
}

func (SecuritySecret) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "security_secrets"},
	}
}

func (SecuritySecret) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
	}
}

func (SecuritySecret) Fields() []ent.Field {
	return []ent.Field{
		field.String("key").
			MaxLen(100).
			NotEmpty().
			Unique(),
		field.String("value").
			NotEmpty().
			SchemaType(map[string]string{
				dialect.Postgres: "text",
			}),
	}
}
