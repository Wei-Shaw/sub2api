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

// PaymentChannel holds the schema definition for the PaymentChannel entity.
//
// 删除策略：硬删除
// PaymentChannel 使用硬删除而非软删除，原因如下：
//   - 渠道配置为管理员维护的元数据，删除即表示不再使用
//   - 通过 enabled 字段控制是否启用，删除仅用于彻底移除
//   - 保持查询简洁，无需额外的软删除过滤
type PaymentChannel struct {
	ent.Schema
}

func (PaymentChannel) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "payment_channels"},
	}
}

func (PaymentChannel) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("group_id").
			Optional().
			Nillable(),
		field.String("name").
			MaxLen(100).
			NotEmpty(),
		field.String("platform").
			MaxLen(50).
			Default("claude"),
		field.Float("rate_multiplier").
			SchemaType(map[string]string{dialect.Postgres: "decimal(10,4)"}).
			Default(1.0),
		field.String("description").
			SchemaType(map[string]string{dialect.Postgres: "text"}).
			Default(""),
		field.String("models").
			SchemaType(map[string]string{dialect.Postgres: "text"}).
			Default(""),
		field.String("features").
			SchemaType(map[string]string{dialect.Postgres: "text"}).
			Default(""),
		field.Int("sort_order").
			Default(0),
		field.Bool("enabled").
			Default(true),
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

func (PaymentChannel) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("group_id"),
		index.Fields("enabled"),
	}
}
