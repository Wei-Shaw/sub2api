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

// BalanceOperation 记录通过 openapi 接口对用户余额做的每一次调整。
// external_op_id 作为外部平台幂等键，唯一索引；同时提供完整审计轨迹。
type BalanceOperation struct {
	ent.Schema
}

func (BalanceOperation) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "balance_operations"},
	}
}

func (BalanceOperation) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
	}
}

func (BalanceOperation) Fields() []ent.Field {
	return []ent.Field{
		field.String("external_op_id").
			MaxLen(128).
			NotEmpty().
			Unique().
			Comment("外部平台传入的操作号，幂等键"),
		field.Int64("user_id").
			Comment("目标用户"),
		field.String("op_type").
			MaxLen(8).
			Comment("set 或 add"),
		field.Float("amount").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}).
			Comment("操作金额"),
		field.Float("balance_before").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}).
			Default(0).
			Comment("执行前余额"),
		field.Float("balance_after").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}).
			Default(0).
			Comment("执行后余额"),
		field.String("status").
			MaxLen(16).
			Default("pending").
			Comment("pending / succeeded / failed"),
		field.String("failure_reason").
			MaxLen(255).
			Optional().
			Nillable(),
		field.String("note").
			MaxLen(255).
			Optional().
			Nillable(),
		field.JSON("request_payload", map[string]any{}).
			Optional().
			Comment("入参快照，审计用"),
	}
}

func (BalanceOperation) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id", "created_at"),
		index.Fields("status"),
	}
}
