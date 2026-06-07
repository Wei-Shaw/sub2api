package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

// RechargePromoActivity 是充值赠送活动表（CRUD 列表语义）。
//
// 与早期"历史不可变快照"模型不同，本表面向管理员的活动列表 UI：
//   - 每行代表一个活动条目，admin 可 List / Create / Update / Delete；
//   - `enabled` 字段控制活动开启状态，全表至多一行 enabled=TRUE
//     （由 partial unique index 在 DB 层兜底，业务层 SetEnabled 在事务内
//     先关掉旧的再启用新的）；
//   - 当前生效活动 = WHERE enabled = TRUE LIMIT 1（再叠加 IsActiveAt 时间窗）；
//   - 支付订单 `activity_id` 外键弱引用本表，命中赠送时记录命中的具体活动；
//   - 前端红点 dismiss key = `id:updated_at_unix`，编辑同一活动会刷新 dismiss key。
//
// 审计追踪由 PaymentAuditLog 承担（每次 update / delete / toggle 单写一条）。
// 删除策略：硬删除——orders.activity_id 是无外键的弱引用，历史订单不受影响。
type RechargePromoActivity struct {
	ent.Schema
}

func (RechargePromoActivity) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "recharge_promo_activities"},
	}
}

func (RechargePromoActivity) Fields() []ent.Field {
	return []ent.Field{
		// 列表 UI 上的可读名称（管理员区分多个活动用）。
		field.String("name").
			MaxLen(120).
			NotEmpty().
			Default("默认活动"),

		// 是否启用：DB 层有 partial unique index 保证全表至多一行为 TRUE。
		field.Bool("enabled").Default(false),

		// 活动有效期窗口；任意一端可为空表示无下限/无上限。
		field.Time("valid_from").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("valid_until").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),

		// 档位整存：[{min_amount, bonus_rate}, ...]，按 min_amount 升序。
		// 类型放在 internal/domain（参考 announcement.targeting / group.* 的做法）：
		// entc 要求 field.JSON 的自定义类型不能与 schema 同包，否则
		// 会触发 schema → mixins → intercept → ent → schema 的循环。
		field.JSON("tiers", []domain.RechargePromoTier{}).
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}),

		// 操作人：admin 用户的展示名/邮箱；写自动化任务时填 system。
		field.String("operator").
			MaxLen(100).
			Default("system"),

		// 备注（admin 可选填写）。
		field.String("note").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "text"}),

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

func (RechargePromoActivity) Indexes() []ent.Index {
	return []ent.Index{
		// 列表 UI 默认按 created_at 倒序翻页。
		index.Fields("created_at"),
		// 全表至多一行 enabled=TRUE（partial unique）。
		index.Fields("enabled").
			Unique().
			Annotations(entsql.IndexWhere("enabled = TRUE")),
	}
}
