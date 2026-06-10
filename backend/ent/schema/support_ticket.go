package schema

import (
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// SupportTicket holds the schema definition for the SupportTicket entity.
//
// 工单生命周期：open → in_progress → closed（终态）。
// 删除策略：硬删除（关联回复通过外键级联）。
// FK 约束（user_id → users.id ON DELETE CASCADE）写在 SQL migration 里，
// 此处不通过 ent edge 显式表达，避免改动 User schema 引发不必要的回归。
type SupportTicket struct {
	ent.Schema
}

func (SupportTicket) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "support_tickets"},
	}
}

func (SupportTicket) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
	}
}

func (SupportTicket) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("user_id").
			Comment("提交工单的用户 ID"),
		field.String("title").
			MaxLen(200).
			NotEmpty().
			Comment("工单标题"),
		field.String("content").
			SchemaType(map[string]string{dialect.Postgres: "text"}).
			NotEmpty().
			Comment("工单首次提交的正文（Markdown）"),
		field.String("category").
			MaxLen(50).
			NotEmpty().
			Comment("工单分类（值来源于 settings.support_ticket_categories 集合）"),
		field.String("status").
			MaxLen(20).
			Default("open").
			Comment("状态: open | in_progress | closed"),
		field.String("priority").
			MaxLen(20).
			Default("normal").
			Comment("优先级: low | normal | high"),
		field.String("chat_context").
			SchemaType(map[string]string{dialect.Postgres: "text"}).
			Optional().
			Nillable().
			Comment("可选的对话上下文快照（浮窗带过来的不可解析文本，最大 50000 字符）"),
		field.Time("closed_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}).
			Comment("关闭时间，仅在 status = closed 时填写"),
	}
}

func (SupportTicket) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("replies", SupportTicketReply.Type).
			Annotations(entsql.OnDelete(entsql.Cascade)),
	}
}

func (SupportTicket) Indexes() []ent.Index {
	return []ent.Index{
		// 用户工单列表（首页 / 我的工单）：按用户 + 状态 + 时间倒序。
		index.Fields("user_id", "status", "created_at"),
		// admin 列表的过滤 + 排序索引：status × priority × 时间。
		index.Fields("status", "priority", "created_at"),
	}
}
