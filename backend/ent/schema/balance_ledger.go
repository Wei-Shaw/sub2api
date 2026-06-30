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

// BalanceLedger 是余额 RPC 的永久流水账本（钱的真值 + 审计）。
//
// 每笔扣费/退费落一行，永不清理（区别于会过期的 idempotency_records）。
//   - kind=1 (deduct)：amount 为本次扣费金额（正数），refunded_amount 累计已退。
//   - kind=2 (refund)：amount 为本次退费金额（正数），refund_of 指向被冲销的原扣 request_id。
//
// 幂等键为 (app_id, request_id)：deduct 用调用方的 request_id，refund 用调用方的 refund_request_id。
type BalanceLedger struct {
	ent.Schema
}

func (BalanceLedger) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "balance_ledger"},
	}
}

func (BalanceLedger) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
	}
}

func (BalanceLedger) Fields() []ent.Field {
	return []ent.Field{
		// 调用方提供的本笔幂等键（deduct=request_id，refund=refund_request_id）
		field.String("request_id").
			MaxLen(128).
			NotEmpty(),
		// 归属接入方
		field.String("app_id").
			MaxLen(64).
			NotEmpty(),
		field.Int64("user_id"),
		// 1=deduct, 2=refund
		field.Int8("kind"),
		// 本笔金额（正数，方向由 kind 决定）。与 users.balance 对齐 decimal(20,8)。
		field.Float("amount").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}),
		// 仅 deduct 行使用：累计已退金额（部分退累加）。
		field.Float("refunded_amount").
			Default(0).
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}),
		// 仅 refund 行使用：被冲销的原 deduct 的 request_id。
		field.String("refund_of").
			MaxLen(128).
			Optional().
			Nillable(),
		// 本笔原因（扣费/退费都必填）。
		field.String("description").
			NotEmpty().
			SchemaType(map[string]string{dialect.Postgres: "text"}),
		// 接入方自存数据（任意 JSON）。
		field.String("extra").
			Default("{}").
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}),
		// 本笔后用户余额快照（审计）。
		field.Float("balance_after").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}),
	}
}

func (BalanceLedger) Indexes() []ent.Index {
	return []ent.Index{
		// 扣/退幂等：同一 app 的 request_id 唯一。
		index.Fields("app_id", "request_id").Unique(),
		// 对账查询。
		index.Fields("user_id", "created_at"),
		// 按原扣聚合退款。
		index.Fields("refund_of"),
	}
}
