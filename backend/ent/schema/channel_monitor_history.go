package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// ChannelMonitorHistory holds the schema definition for the ChannelMonitorHistory entity.
// 渠道监控历史：每次检测每个模型一行记录。明细只保留 1 天，超过 1 天由每日维护任务
// 先聚合到 channel_monitor_daily_rollups，再分批物理删（不用软删除：日志类表无恢复
// 需求，软删会让行和索引只增不减，徒增磁盘和查询开销）。
type ChannelMonitorHistory struct {
	ent.Schema
}

func (ChannelMonitorHistory) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "channel_monitor_histories"},
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
