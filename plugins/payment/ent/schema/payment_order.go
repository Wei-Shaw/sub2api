package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/shopspring/decimal"
)

// PaymentOrder holds the schema definition for the PaymentOrder entity.
//
// 删除策略：硬删除
// PaymentOrder 使用硬删除而非软删除，原因如下：
//   - 订单通过 status 字段追踪完整生命周期，无需依赖软删除
//   - 订单审计通过 PaymentAuditLog 表记录，删除前可归档
//   - 减少查询复杂度，避免软删除过滤开销
type PaymentOrder struct {
	ent.Schema
}

func (PaymentOrder) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "payment_orders"},
	}
}

func (PaymentOrder) Fields() []ent.Field {
	return []ent.Field{
		// 用户信息（冗余存储，避免关联查询）
		field.Int64("user_id"),
		field.String("user_email").
			MaxLen(255),
		field.String("user_name").
			MaxLen(100),
		field.String("user_notes").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "text"}),

		// 金额信息 — 使用 shopspring/decimal 保证金融级精度,避免 float64 在累加 / 乘除时累积误差
		field.Other("amount", decimal.Decimal{}).
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,2)"}),
		field.Other("pay_amount", decimal.Decimal{}).
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,2)"}),
		field.Other("fee_rate", decimal.Decimal{}).
			SchemaType(map[string]string{dialect.Postgres: "decimal(10,4)"}).
			Default(decimal.Zero),
		field.String("recharge_code").
			MaxLen(64),

		// 支付信息
		field.String("out_trade_no").
			MaxLen(64).
			Default(""),
		field.String("payment_type").
			MaxLen(30),
		field.String("payment_trade_no").
			MaxLen(128),
		field.String("pay_url").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "text"}),
		field.String("qr_code").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "text"}),
		field.String("qr_code_img").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "text"}),

		// 订单类型 & 订阅关联
		field.String("order_type").
			MaxLen(20).
			Default("balance"),
		field.Int64("plan_id").
			Optional().
			Nillable(),
		field.Int64("subscription_group_id").
			Optional().
			Nillable(),
		field.Int("subscription_days").
			Optional().
			Nillable(),
		field.String("provider_instance_id").
			Optional().
			Nillable().
			MaxLen(64),
		field.String("provider_key").
			Optional().
			Nillable().
			MaxLen(30),
		field.JSON("provider_snapshot", map[string]any{}).
			Optional().
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}),

		// 状态
		field.String("status").
			MaxLen(30).
			Default("PENDING"),

		// 退款信息 — 同样用 decimal 保证精度
		field.Other("refund_amount", decimal.Decimal{}).
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,2)"}).
			Default(decimal.Zero),
		field.String("refund_reason").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "text"}),
		field.Time("refund_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Bool("force_refund").
			Default(false),
		field.Time("refund_requested_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.String("refund_request_reason").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "text"}),
		field.String("refund_requested_by").
			Optional().
			Nillable().
			MaxLen(20),

		// 时间节点
		field.Time("expires_at").
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("paid_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("completed_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("failed_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.String("failed_reason").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "text"}),

		// 来源信息
		field.String("client_ip").
			MaxLen(50),
		field.String("src_host").
			MaxLen(255),
		field.String("src_url").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "text"}),

		// 时间戳
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

// Edges returns no edges. The User entity lives in the host's ent schema
// (not the plugin's), so the user_id field is kept as a plain int64 and
// joins cross-plugin via the SDK HostPaymentClient.GetUserByID lookup
// rather than an in-plugin foreign key.
func (PaymentOrder) Edges() []ent.Edge {
	return nil
}

func (PaymentOrder) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("out_trade_no").
			Unique().
			Annotations(entsql.IndexWhere("out_trade_no <> ''")),
		index.Fields("user_id"),
		index.Fields("status"),
		index.Fields("expires_at"),
		index.Fields("created_at"),
		index.Fields("paid_at"),
		index.Fields("payment_type", "paid_at"),
		index.Fields("order_type"),
	}
}
