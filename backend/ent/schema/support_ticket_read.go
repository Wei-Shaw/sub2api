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

// SupportTicketRead holds the schema definition for a per-(ticket,user) read cursor.
//
// 语义：记录某用户（工单作者 or 处理该工单的 admin）最后一次查看/回复该工单详情的时刻。
// 用于聚合"未读工单数"（有 admin 回复 or 用户回复晚于 last_read_at 即算未读）。
//
// FK 约束（ticket_id → support_tickets.id ON DELETE CASCADE、
// user_id → users.id ON DELETE CASCADE）写在 SQL migration 里，
// 此处不通过 ent edge 显式表达，跟 SupportTicket / SupportTicketReply 保持一致，
// 避免改动 User / SupportTicket schema 引发不必要的回归。
type SupportTicketRead struct {
	ent.Schema
}

func (SupportTicketRead) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "support_ticket_reads"},
	}
}

func (SupportTicketRead) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
	}
}

func (SupportTicketRead) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("ticket_id").
			Comment("所属工单 ID"),
		field.Int64("user_id").
			Comment("已读用户 ID（工单作者 or admin）"),
		field.Time("last_read_at").
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}).
			Comment("该用户最后一次读取该工单详情的时间"),
	}
}

func (SupportTicketRead) Indexes() []ent.Index {
	return []ent.Index{
		// (ticket_id, user_id) 唯一：一个用户对同一工单只有一个 read 游标。
		index.Fields("ticket_id", "user_id").Unique(),
		// 未读聚合查询走该索引：按用户扫最近读过的工单集合。
		index.Fields("user_id", "last_read_at"),
	}
}
