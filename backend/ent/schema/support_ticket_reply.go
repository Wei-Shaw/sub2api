package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// SupportTicketReply holds the schema definition for one ticket reply (user or admin).
//
// FK 约束（author_id → users.id ON DELETE SET NULL）写在 SQL migration 里。
// is_admin 是写入时角色快照——不实时算，避免 admin 降权后历史回复丢失"权威"标识。
type SupportTicketReply struct {
	ent.Schema
}

func (SupportTicketReply) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "support_ticket_replies"},
	}
}

func (SupportTicketReply) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("ticket_id").
			Comment("所属工单 ID"),
		field.Int64("author_id").
			Optional().
			Nillable().
			Comment("回复作者用户 ID（用户被删除时置 NULL）"),
		field.Bool("is_admin").
			Default(false).
			Comment("是否为 admin 回复（写入时按调用路由快照）"),
		field.String("content").
			SchemaType(map[string]string{dialect.Postgres: "text"}).
			NotEmpty().
			Comment("回复正文（Markdown）"),
		field.Time("created_at").
			Immutable().
			Default(time.Now).
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (SupportTicketReply) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("ticket", SupportTicket.Type).
			Ref("replies").
			Field("ticket_id").
			Unique().
			Required(),
	}
}

func (SupportTicketReply) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("ticket_id", "created_at"),
	}
}
