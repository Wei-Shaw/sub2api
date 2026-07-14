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

// SupportChatConversation holds the schema definition for a support-chat conversation header.
//
// 客服浮窗对话审计（add-support-chat-transcript-log）：以浏览器已在发送的 session_id
// 为业务键，把同一段对话的多轮问答归并到一个会话头。turn_count 每轮 +1。
// 删除策略：硬删除（关联消息通过外键级联）。
// FK 约束（user_id → users.id ON DELETE SET NULL，匿名对话为 NULL）写在 SQL migration 里，
// 此处不通过 ent edge 显式表达，避免改动 User schema 引发不必要的回归。
type SupportChatConversation struct {
	ent.Schema
}

func (SupportChatConversation) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "support_chat_conversations"},
	}
}

func (SupportChatConversation) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
	}
}

func (SupportChatConversation) Fields() []ent.Field {
	return []ent.Field{
		field.String("session_id").
			MaxLen(128).
			NotEmpty().
			Unique().
			Comment("客户端生成的会话键，唯一，用于幂等 upsert 归并同一段对话"),
		field.Int64("user_id").
			Optional().
			Nillable().
			Comment("登录用户 ID；匿名对话为 NULL（用户删除时置 NULL）"),
		field.String("client_ip").
			MaxLen(64).
			Optional().
			Comment("发起对话的客户端 IP"),
		field.Int("turn_count").
			Default(0).
			Comment("已归并的问答轮数（每轮 +1）"),
		field.String("last_status").
			MaxLen(24).
			Optional().
			Comment("最近一轮 assistant 状态: success | upstream_auth | upstream_error | interrupted | rate_limited | config_error"),
		field.Time("first_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}).
			Comment("首轮时间"),
		field.Time("last_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}).
			Comment("最近一轮时间"),
	}
}

func (SupportChatConversation) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("messages", SupportChatMessage.Type).
			Annotations(entsql.OnDelete(entsql.Cascade)),
	}
}

func (SupportChatConversation) Indexes() []ent.Index {
	return []ent.Index{
		// admin 按状态过滤 + 时间倒序
		index.Fields("last_status", "last_at"),
		// admin 按用户查 + 时间倒序
		index.Fields("user_id", "last_at"),
	}
}
