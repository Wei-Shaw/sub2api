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

// PaymentAuditLog holds the schema definition for the PaymentAuditLog entity.
//
// 删除策略：硬删除
// PaymentAuditLog 使用硬删除而非软删除，原因如下：
//   - 审计日志本身即为不可变记录，通常只追加不修改
//   - 如需清理历史日志，直接按时间范围批量删除即可
//   - 保持表结构简洁，提升插入和查询性能
type PaymentAuditLog struct {
	ent.Schema
}

func (PaymentAuditLog) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "payment_audit_logs"},
	}
}

func (PaymentAuditLog) Fields() []ent.Field {
	return []ent.Field{
		field.String("order_id").
			MaxLen(64),
		field.String("action").
			MaxLen(50),
		field.String("detail").
			SchemaType(map[string]string{dialect.Postgres: "text"}).
			Default(""),
		field.String("operator").
			MaxLen(100).
			Default("system"),
		field.Time("created_at").
			Immutable().
			Default(time.Now).
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (PaymentAuditLog) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("order_id"),
	}
}
