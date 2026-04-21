package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"

	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"
)

// ChannelMonitorHistory holds the schema definition for the ChannelMonitorHistory entity.
// 渠道监控历史：每次检测每个模型一行记录。明细只保留 1 天，超过 1 天的数据被聚合到
// channel_monitor_daily_rollups 后软删（deleted_at），由后续懒清理任务物理移除。
type ChannelMonitorHistory struct {
	ent.Schema
}

func (ChannelMonitorHistory) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "channel_monitor_histories"},
	}
}

func (ChannelMonitorHistory) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.SoftDeleteMixin{},
	}
}

func (ChannelMonitorHistory) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("monitor_id"),
		field.String("model").
			NotEmpty().
			MaxLen(200),
		field.Enum("status").
			Values("operational", "degraded", "failed", "error"),
		field.Int("latency_ms").
			Optional().
			Nillable(),
		field.Int("ping_latency_ms").
			Optional().
			Nillable(),
		field.String("message").
			Optional().
			Default("").
			MaxLen(500),
		field.Time("checked_at").
			Default(time.Now),
	}
}

func (ChannelMonitorHistory) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("monitor", ChannelMonitor.Type).
			Ref("history").
			Field("monitor_id").
			Unique().
			Required(),
	}
}

func (ChannelMonitorHistory) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("monitor_id", "model", "checked_at"),
		index.Fields("checked_at"),
	}
}
