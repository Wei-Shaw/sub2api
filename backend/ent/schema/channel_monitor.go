package schema

import (
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// ChannelMonitor holds the schema definition for the ChannelMonitor entity.
// 渠道监控配置：定期对指定 provider/endpoint/api_key 下的模型做心跳测试。
type ChannelMonitor struct {
	ent.Schema
}

func (ChannelMonitor) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "channel_monitors"},
	}
}

func (ChannelMonitor) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
	}
}

func (ChannelMonitor) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").
			NotEmpty().
			MaxLen(100),
		field.Enum("provider").
			Values("openai", "anthropic", "gemini"),
		field.String("endpoint").
			NotEmpty().
			MaxLen(500).
			Comment("Provider base origin, e.g. https://api.openai.com"),
		field.String("api_key_encrypted").
			NotEmpty().
			Sensitive().
			Comment("AES-256-GCM encrypted API key"),
		field.String("primary_model").
			NotEmpty().
			MaxLen(200),
		field.JSON("extra_models", []string{}).
			Default([]string{}).
			Comment("Additional model names to test alongside primary_model"),
		field.String("group_name").
			Optional().
			Default("").
			MaxLen(100),
		field.Bool("enabled").
			Default(true),
		field.Int("interval_seconds").
			Range(15, 3600),
		field.Time("last_checked_at").
			Optional().
			Nillable(),
		field.Int64("created_by"),

		// ---- 自定义请求快照字段（来自模板 / 手动编辑） ----

		// template_id: 关联的请求模板 ID（仅用于 UI 分组 + 一键应用）。
		// 实际运行时 checker 只读下面 3 个快照字段，**不再回查模板表**。
		// 模板被删除时此字段会被 SET NULL（见 Edges 的 OnDelete 注解）。
		field.Int64("template_id").
			Optional().
			Nillable(),
		// extra_headers: 自定义 HTTP 头快照（来自模板 or 用户手填）。
		// 运行时 merge 进 adapter 默认 headers。
		field.JSON("extra_headers", map[string]string{}).
			Default(map[string]string{}),
		// body_override_mode: 同 ChannelMonitorRequestTemplate.body_override_mode
		field.String("body_override_mode").
			Default("off").
			MaxLen(10),
		// body_override: 同 ChannelMonitorRequestTemplate.body_override
		field.JSON("body_override", map[string]any{}).
			Optional(),
	}
}

func (ChannelMonitor) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("history", ChannelMonitorHistory.Type).
			Annotations(entsql.OnDelete(entsql.Cascade)),
		edge.To("daily_rollups", ChannelMonitorDailyRollup.Type).
			Annotations(entsql.OnDelete(entsql.Cascade)),
		// 关联请求模板：模板被删除时 template_id 自动置空，
		// 监控本身保留（继续用快照字段跑）。
		edge.To("request_template", ChannelMonitorRequestTemplate.Type).
			Field("template_id").
			Unique().
			Annotations(entsql.OnDelete(entsql.SetNull)),
	}
}

func (ChannelMonitor) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("enabled", "last_checked_at"),
		index.Fields("provider"),
		index.Fields("group_name"),
		index.Fields("template_id"),
	}
}
