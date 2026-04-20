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
	}
}

func (ChannelMonitor) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("history", ChannelMonitorHistory.Type).
			Annotations(entsql.OnDelete(entsql.Cascade)),
	}
}

func (ChannelMonitor) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("enabled", "last_checked_at"),
		index.Fields("provider"),
		index.Fields("group_name"),
	}
}
