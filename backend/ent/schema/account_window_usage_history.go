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

// AccountWindowUsageHistory retains observed OpenAI OAuth quota periods and local
// API-price comparisons. Its journal is SQL-managed and consumed transactionally.
type AccountWindowUsageHistory struct {
	ent.Schema
}

func (AccountWindowUsageHistory) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "account_window_usage_histories"},
	}
}

// Mixin 返回该 schema 使用的混入组件。
// - TimeMixin: 自动管理 created_at 和 updated_at 时间戳
func (AccountWindowUsageHistory) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
	}
}

func (AccountWindowUsageHistory) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("account_id"),
		// Only ordinary OpenAI 5h / 7d quota windows are recorded.
		field.String("window_type").
			NotEmpty().
			MaxLen(32),
		// window_end is the observed reset boundary, or an early-reset cut point.
		// reset_at separately retains the provider boundary used for late samples.
		field.Time("window_start").
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("window_end").
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		// peak/last_used_percent: 窗口内峰值/最新使用率（0-100+，不截断，
		// >100 正是限额缩水调查需要看到的原始值）
		field.Float("peak_used_percent").
			Default(0),
		field.Float("last_used_percent").
			Default(0),
		// Each journal id is processed exactly once under the account row lock.
		// Distinct observations may legitimately share the same timestamp.
		field.Int("sample_count").
			Default(0),
		// last_sample_at: 行内最新采样的观测时刻（快照抓取时间），单调前移
		field.Time("last_sample_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		// Locally recorded cumulative usage at the latest sample; refreshed once
		// more after the closed period's late-write grace interval.
		field.Int64("requests").Default(0),
		field.Int64("tokens_total").Default(0),
		field.Time("reset_at").SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Int("duration_minutes"),
		field.Time("first_observed_at").SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Int64("last_observation_id").Default(0),
		field.Float("api_reference_cost").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "numeric(20,10)"}),
		field.Int64("priced_requests").Default(0),
		field.Int64("missing_pricing_requests").Default(0),
		field.Float("estimated_reference_limit").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "numeric(20,10)"}),
		field.Float("estimate_reference_cost").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "numeric(20,10)"}),
		field.Float("estimate_used_percent").Optional().Nillable(),
		field.Time("estimate_observed_at").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.JSON("quality_flags", []string{}).Default([]string{}),
		field.String("end_reason").MaxLen(32).Optional().Nillable(),
		field.Time("stats_finalized_at").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		// finalized_at: 窗口关闭时间；NULL = 开放行（当前窗口）
		field.Time("finalized_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (AccountWindowUsageHistory) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("account", Account.Type).
			Ref("window_usage_histories").
			Field("account_id").
			Unique().
			Required(),
	}
}

func (AccountWindowUsageHistory) Indexes() []ent.Index {
	return []ent.Index{
		// 每账号每窗口类型至多一行开放记录（upsert 冲突目标）
		index.Fields("account_id", "window_type").
			Unique().
			Annotations(entsql.IndexWhere("finalized_at IS NULL")),
		// 管理端统计弹窗的历史查询
		index.Fields("account_id", "window_type", "window_end"),
		// finalize 扫描
		index.Fields("window_end").
			Annotations(entsql.IndexWhere("finalized_at IS NULL")),
	}
}
