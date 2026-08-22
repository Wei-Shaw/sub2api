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

// AccountWindowUsageHistory holds the schema definition for the AccountWindowUsageHistory entity.
// 账号滚动窗口用量历史：纯被动统计每个账号各滚动窗口（5h/7d/7d-sonnet/7d-fable/weekly）
// 的使用率曲线——观测全部来自既有数据流（Codex 流量头快照、渠道监控明细历史），
// 不产生任何主动探测。窗口关闭（finalized_at 非空）后由 usage_logs 重建该窗口的
// token 明细。每账号每窗口类型至多一行未关闭记录，由局部唯一索引
// (account_id, window_type) WHERE finalized_at IS NULL 保证（upsert 冲突目标）。
// 明细按保留期物理删除（日志类表，不用软删除）。
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
		// window_type: 滚动窗口类型 token，复用 domain.MonitorQuotaTier.Window 取值
		// （"5h" / "7d" / "7d-sonnet" / "7d-fable" / "weekly"）
		field.String("window_type").
			NotEmpty().
			MaxLen(32),
		// window_end: 最后观测到的 reset_at（滚动窗口会向前滑动，关闭时定格）；
		// window_start = window_end - duration(window_type)，随滑动同步重算
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
		// sample_count: 累计采样次数；同一观测（last_sample_at 相同或更早）
		// 重复回放时恰好计数一次（多副本 / 重启回填幂等）
		field.Int("sample_count").
			Default(0),
		// last_sample_at: 行内最新采样的观测时刻（快照抓取时间），单调前移
		field.Time("last_sample_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		// requests / tokens_*: 窗口关闭后由 usage_logs 在 [window_start, window_end)
		// 内聚合回填；finalized_at 为空时均为 NULL
		field.Int64("requests").
			Optional().
			Nillable(),
		field.Int64("tokens_total").
			Optional().
			Nillable(),
		field.Int64("tokens_input").
			Optional().
			Nillable(),
		field.Int64("tokens_output").
			Optional().
			Nillable(),
		field.Int64("tokens_cache_creation").
			Optional().
			Nillable(),
		field.Int64("tokens_cache_read").
			Optional().
			Nillable(),
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
