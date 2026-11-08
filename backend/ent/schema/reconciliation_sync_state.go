package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
)

// ReconciliationSyncState 保存「经营对账」的同步游标与运行状态。
//
// 形态与 settings 表一致（key 唯一 + value 文本），但用途完全不同：
// settings 面向用户可改的配置，本表面向采集器的内部进度，不应出现在设置界面。
//
// 键约定（见设计文档 §2.4）：
//
//	usage_last_collected_at        下游用量采集游标（RFC3339Nano UTC）
//	a6_last_sync_unix              A6 上次成功同步时间（Unix 秒）
//	a6_last_sync_error             A6 上次同步错误摘要
//	a6_last_sync_error_at          上述错误发生时间
//	a6_bootstrap_done:<token>      某 A6 令牌是否完成过首次同步
//	a6_backfill_status            历史回填状态：running / completed / failed
//	a6_backfill_from/_to/_cursor/_processed/_error
//
// 删除策略：硬删除。游标类状态被删除即等价于「重新开始采集」，语义清晰。
type ReconciliationSyncState struct {
	ent.Schema
}

func (ReconciliationSyncState) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "reconciliation_sync_state"},
	}
}

func (ReconciliationSyncState) Fields() []ent.Field {
	return []ent.Field{
		field.String("key").
			MaxLen(100).
			NotEmpty().
			Unique(),
		field.String("value").
			Default("").
			SchemaType(map[string]string{
				dialect.Postgres: "text",
			}),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now).
			SchemaType(map[string]string{
				dialect.Postgres: "timestamptz",
			}),
	}
}

func (ReconciliationSyncState) Indexes() []ent.Index {
	// key 字段已在 Fields() 中声明 Unique()，无需额外索引
	return nil
}
