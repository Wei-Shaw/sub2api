package schema

import (
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// UpstreamBalanceMonitor stores an independently scheduled upstream balance probe.
type UpstreamBalanceMonitor struct {
	ent.Schema
}

func (UpstreamBalanceMonitor) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "upstream_balance_monitors"}}
}

func (UpstreamBalanceMonitor) Mixin() []ent.Mixin {
	return []ent.Mixin{mixins.TimeMixin{}}
}

func (UpstreamBalanceMonitor) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").NotEmpty().MaxLen(100),
		field.Enum("type").Values("sub2api", "newapi"),
		field.String("base_url").NotEmpty().MaxLen(500),
		field.String("api_key_encrypted").NotEmpty().Sensitive(),
		field.Bool("enabled").Default(true),
		field.Int("display_order").Default(0),
		field.Int("probe_interval_minutes").Default(30).Range(5, 1440),
		field.Float("low_balance_threshold_usd").Default(10).Min(0),
		field.Time("last_probe_at").Optional().Nillable(),
		field.String("last_probe_status").Default("pending").MaxLen(16),
		field.String("last_probe_error").Optional().Nillable(),
		field.JSON("snapshot_data", map[string]any{}).Default(map[string]any{}),
		field.Time("next_probe_at").Optional().Nillable(),
		field.Int("failure_count").Default(0).Min(0),
	}
}

func (UpstreamBalanceMonitor) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("enabled", "next_probe_at"),
		index.Fields("display_order", "id"),
	}
}
