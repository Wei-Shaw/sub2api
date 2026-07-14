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

// SupportChatMessage holds the schema definition for one turn's message row.
//
// 客服浮窗对话审计（add-support-chat-transcript-log）：一轮问答落两行——
// user 行（role=user，无 status）+ assistant 回包行（role=assistant，带完整状态分类）。
// 即使回包失败（如 upstream_auth），user 行仍留痕，能看到"用户问了但没答上"。
// FK 约束（conversation_id → support_chat_conversations.id ON DELETE CASCADE）写在 SQL migration 里。
type SupportChatMessage struct {
	ent.Schema
}

func (SupportChatMessage) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "support_chat_messages"},
	}
}

func (SupportChatMessage) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("conversation_id").
			Comment("所属会话 ID"),
		field.String("role").
			MaxLen(16).
			NotEmpty().
			Comment("角色: user | assistant"),
		field.String("content").
			SchemaType(map[string]string{dialect.Postgres: "text"}).
			Comment("消息正文；落库前由 service 层截断到 50000 字符"),
		field.String("status").
			MaxLen(24).
			Optional().
			Comment("assistant 行状态: success | upstream_auth | upstream_error | interrupted | rate_limited | config_error（user 行为空）"),
		field.String("error_message").
			SchemaType(map[string]string{dialect.Postgres: "text"}).
			Optional().
			Comment("失败细节（status != success 时填）"),
		field.String("model").
			MaxLen(128).
			Optional().
			Comment("本轮使用的上游模型"),
		field.Int("latency_ms").
			Optional().
			Nillable().
			Comment("本轮耗时毫秒（handler 进入到流收尾）"),
		field.Time("created_at").
			Immutable().
			Default(time.Now).
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (SupportChatMessage) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("conversation", SupportChatConversation.Type).
			Ref("messages").
			Field("conversation_id").
			Unique().
			Required(),
	}
}

func (SupportChatMessage) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("conversation_id", "created_at"),
	}
}
