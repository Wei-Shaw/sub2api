package schema

import (
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// EmailBroadcast 管理员批量公告邮件记录。
// EmailBroadcast records an admin-issued bulk announcement email batch.
type EmailBroadcast struct {
	ent.Schema
}

func (EmailBroadcast) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "email_broadcasts"},
	}
}

func (EmailBroadcast) Fields() []ent.Field {
	return []ent.Field{
		field.String("subject").
			MaxLen(200).
			NotEmpty().
			Comment("邮件主题"),
		field.String("body").
			SchemaType(map[string]string{dialect.Postgres: "text"}).
			NotEmpty().
			Comment("邮件正文 (HTML 或纯文本)"),
		field.String("body_format").
			MaxLen(10).
			Default(domain.EmailBroadcastBodyFormatHTML).
			Comment("正文格式: html, text"),
		field.String("recipients_mode").
			MaxLen(20).
			Default(domain.EmailBroadcastRecipientsModeSelected).
			Comment("收件人模式: all, selected"),
		field.JSON("recipient_user_ids", []int64{}).
			Optional().
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}).
			Comment("收件人用户 ID 列表 (selected 模式时使用)"),
		field.String("status").
			MaxLen(20).
			Default(domain.EmailBroadcastStatusPending).
			Comment("状态: pending, sending, completed, failed"),
		field.Int("total_count").
			Default(0).
			NonNegative().
			Comment("收件人总数"),
		field.Int("success_count").
			Default(0).
			NonNegative().
			Comment("发送成功数"),
		field.Int("failed_count").
			Default(0).
			NonNegative().
			Comment("发送失败数"),
		field.String("error_message").
			SchemaType(map[string]string{dialect.Postgres: "text"}).
			Optional().
			Nillable().
			Comment("整批失败原因 (status=failed 时填充)"),
		field.Int64("created_by").
			Optional().
			Nillable().
			Comment("创建管理员用户 ID"),
		field.Time("started_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}).
			Comment("开始发送时间"),
		field.Time("finished_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}).
			Comment("发送结束时间"),
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

func (EmailBroadcast) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("status"),
		index.Fields("created_at"),
		index.Fields("created_by"),
	}
}
