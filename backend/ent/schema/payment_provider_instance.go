package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// PaymentProviderInstance holds the schema definition for the PaymentProviderInstance entity.
//
// 删除策略：硬删除
// PaymentProviderInstance 使用硬删除而非软删除，原因如下：
//   - 服务商实例为管理员配置的支付通道，删除即表示废弃
//   - 通过 enabled 字段控制是否启用，删除仅用于彻底移除
//   - config 字段存储加密后的密钥信息，删除时应彻底清除
type PaymentProviderInstance struct {
	ent.Schema
}

func (PaymentProviderInstance) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "payment_provider_instances"},
	}
}

func (PaymentProviderInstance) Fields() []ent.Field {
	return []ent.Field{
		field.String("provider_key").
			MaxLen(30).
			NotEmpty(),
		field.String("name").
			MaxLen(100).
			Default(""),
		field.String("config").
			SchemaType(map[string]string{dialect.Postgres: "text"}),
		field.String("supported_types").
			MaxLen(200).
			Default(""),
		field.Bool("enabled").
			Default(true),
		field.String("payment_mode").
			MaxLen(20).
			Default(""),
		field.Int("sort_order").
			Default(0),
		field.String("limits").
			SchemaType(map[string]string{dialect.Postgres: "text"}).
			Default(""),
		field.Bool("refund_enabled").
			Default(false),
		field.Time("created_at").
			Immutable().
			Default(time.Now).
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now).
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (PaymentProviderInstance) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("provider_key"),
		index.Fields("enabled"),
	}
}
