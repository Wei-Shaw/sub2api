package schema

import (
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Announcement holds the schema definition for the Announcement entity.
//
// 删除策略：硬删除（已读记录通过外键级联删除）
type Announcement struct {
	ent.Schema
}

func (Announcement) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "announcements"},
	}
}

func (Announcement) Fields() []ent.Field {
	return []ent.Field{
		field.String("title").
			MaxLen(200).
			NotEmpty().
			Comment("公告标题"),
		field.String("content").
			SchemaType(map[string]string{dialect.Postgres: "text"}).
			NotEmpty().
			Comment("公告内容（支持 Markdown）"),
		field.String("status").
			MaxLen(20).
			Default(domain.AnnouncementStatusDraft).
			Comment("状态: draft, active, archived"),
		field.String("notify_mode").
			MaxLen(20).
			Default(domain.AnnouncementNotifyModeSilent).
			Comment("通知模式: silent(仅铃铛), popup(弹窗提醒)"),
		field.JSON("targeting", domain.AnnouncementTargeting{}).
			Optional().
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}).
			Comment("展示条件（JSON 规则）"),
		field.Time("starts_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}).
			Comment("开始展示时间（为空表示立即生效）"),
		field.Time("ends_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}).
			Comment("结束展示时间（为空表示永久生效）"),
		field.Int64("created_by").
			Optional().
			Nillable().
			Comment("创建人用户ID（管理员）"),
		field.Int64("updated_by").
			Optional().
			Nillable().
			Comment("更新人用户ID（管理员）"),
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

func (Announcement) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("reads", AnnouncementRead.Type),
	}
}

func (Announcement) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("status"),
		index.Fields("created_at"),
		index.Fields("starts_at"),
		index.Fields("ends_at"),
	}
}
