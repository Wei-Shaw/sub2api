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

// SupportTicketNotification holds the schema definition for a persistent in-app
// notification bound to a support ticket event.
//
// 语义：每次工单生命周期事件（新建 / 用户回复 / 管理员回复）都会为每个 recipient 生成
// 一条通知记录，用于铃铛面板"工单"tab 与未读计数。邮件通道是并行独立的（同事件同 recipient 也发邮件），
// 通知记录只跟站内展示强相关。
//
// FK 约束（ticket_id → support_tickets.id ON DELETE CASCADE、
// actor_user_id → users.id ON DELETE SET NULL、
// recipient_user_id → users.id ON DELETE CASCADE）写在 SQL migration 里，
// 此处不通过 ent edge 显式表达，跟 SupportTicket / SupportTicketReply / SupportTicketRead 保持一致。
type SupportTicketNotification struct {
	ent.Schema
}

func (SupportTicketNotification) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "support_ticket_notification"},
	}
}

func (SupportTicketNotification) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("recipient_user_id").
			Comment("接收该通知的用户 ID"),
		field.Int64("ticket_id").
			Comment("关联工单 ID"),
		field.String("event_type").
			MaxLen(50).
			NotEmpty().
			Comment("事件类型: ticket_created | admin_replied | user_replied"),
		field.String("title_snapshot").
			MaxLen(200).
			NotEmpty().
			Comment("写入时工单标题快照（工单标题后续被改动时通知面板依然显示当时值）"),
		field.String("excerpt").
			MaxLen(500).
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "varchar(500)"}).
			Comment("事件正文摘要（回复内容首 200 字符等）；可空"),
		field.Int64("actor_user_id").
			Optional().
			Nillable().
			Comment("触发事件的用户 ID（用户创建/回复工单 or admin 回复）；用户被删除时置 NULL"),
		field.Bool("is_read").
			Default(false).
			Comment("recipient 是否已读该条通知"),
		field.Time("created_at").
			Immutable().
			Default(time.Now).
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}).
			Comment("通知生成时间"),
		field.Time("read_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}).
			Comment("标记已读的时刻；is_read=true 时非空"),
	}
}

func (SupportTicketNotification) Indexes() []ent.Index {
	return []ent.Index{
		// 铃铛面板主查询：某 recipient 的最新 N 条 + 未读数聚合。
		index.Fields("recipient_user_id", "is_read", "created_at"),
		// ticket 详情内批量标已读 / 级联删除辅助。
		index.Fields("ticket_id"),
	}
}
