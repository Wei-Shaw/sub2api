// Package schema 定义 Ent ORM 的数据库 schema。
// 每个文件对应一个数据库实体（表），定义其字段、边（关联）和索引。
package schema

import (
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"
	"github.com/Wei-Shaw/sub2api/internal/domain"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Account 定义 AI API 账户实体的 schema。
//
// 账户是系统的核心资源，代表一个可用于调用 AI API 的凭证。
// 例如：一个 Claude API 账户、一个 Gemini OAuth 账户等。
//
// 主要功能：
//   - 存储不同平台（Claude、Gemini、OpenAI 等）的 API 凭证
//   - 支持多种认证类型（api_key、oauth、cookie 等）
//   - 管理账户的调度状态（可调度、速率限制、过载等）
//   - 通过分组机制实现账户的灵活分配
type Account struct {
	ent.Schema
}

// Annotations 返回 schema 的注解配置。
// 这里指定数据库表名为 "accounts"。
func (Account) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "accounts"},
	}
}

// Mixin 返回该 schema 使用的混入组件。
// - TimeMixin: 自动管理 created_at 和 updated_at 时间戳
// - SoftDeleteMixin: 提供软删除功能（deleted_at）
func (Account) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
		mixins.SoftDeleteMixin{},
	}
}

// Fields 定义账户实体的所有字段。
func (Account) Fields() []ent.Field {
	return []ent.Field{
		// name: 账户显示名称，用于在界面中标识账户
		field.String("name").
			MaxLen(100).
			NotEmpty(),
		// notes: 管理员备注（可为空）
		field.String("notes").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "text"}),

		// platform: 所属平台，如 "claude", "gemini", "openai" 等
		field.String("platform").
			MaxLen(50).
			NotEmpty(),

		// type: 认证类型，如 "api_key", "oauth", "cookie" 等
		// 不同类型决定了 credentials 中存储的数据结构
		field.String("type").
			MaxLen(20).
			NotEmpty(),

		// credentials: 认证凭证，以 JSONB 格式存储
		// 结构取决于 type 字段：
		// - api_key: {"api_key": "sk-xxx"}
		// - oauth: {"access_token": "...", "refresh_token": "...", "expires_at": "..."}
		// - cookie: {"session_key": "..."}
		field.JSON("credentials", map[string]any{}).
			Default(func() map[string]any { return map[string]any{} }).
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}),

		// extra: 扩展数据，存储平台特定的额外信息
		// 如 CRS 账户的 crs_account_id、组织信息等
		field.JSON("extra", map[string]any{}).
			Default(func() map[string]any { return map[string]any{} }).
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}),

		// proxy_id: 关联的代理配置 ID（可选）
		// 用于需要通过特定代理访问 API 的场景
		field.Int64("proxy_id").
			Optional().
			Nillable(),

		// concurrency: 账户最大并发请求数
		// 用于限制同一时间对该账户发起的请求数量
		field.Int("concurrency").
			Default(3),

		field.Int("load_factor").Optional().Nillable(),

		// priority: 账户优先级，数值越小优先级越高
		// 调度器会优先使用高优先级的账户
		field.Int("priority").
			Default(50),

		// rate_multiplier: 账号计费倍率（>=0，允许 0 表示该账号计费为 0）
		// 仅影响账号维度计费口径，不影响用户/API Key 扣费（分组倍率）
		field.Float("rate_multiplier").
			SchemaType(map[string]string{dialect.Postgres: "decimal(10,4)"}).
			Default(1.0),

		// status: 账户状态，如 "active", "error", "disabled"
		field.String("status").
			MaxLen(20).
			Default(domain.StatusActive),

		// error_message: 错误信息，记录账户异常时的详细信息
		field.String("error_message").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "text"}),

		// last_used_at: 最后使用时间，用于统计和调度
		field.Time("last_used_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		// expires_at: 账户过期时间（可为空）
		field.Time("expires_at").
			Optional().
			Nillable().
			Comment("Account expiration time (NULL means no expiration).").
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		// auto_pause_on_expired: 过期后自动暂停调度
		field.Bool("auto_pause_on_expired").
			Default(true).
			Comment("Auto pause scheduling when account expires."),

		// ========== 调度和速率限制相关字段 ==========
		// 这些字段在 migrations/005_schema_parity.sql 中添加

		// schedulable: 是否可被调度器选中
		// false 表示账户暂时不参与请求分配（如正在刷新 token）
		field.Bool("schedulable").
			Default(true),

		// rate_limited_at: 触发速率限制的时间
		// 当收到 429 错误时记录
		field.Time("rate_limited_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),

		// rate_limit_reset_at: 速率限制预计解除的时间
		// 调度器会在此时间之前避免使用该账户
		field.Time("rate_limit_reset_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),

		// overload_until: 过载状态解除时间
		// 当收到 529 错误（API 过载）时设置
		field.Time("overload_until").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),

		// temp_unschedulable_until: 临时不可调度状态解除时间
		// 当命中临时不可调度规则时设置，在此时间前调度器应跳过该账号
		field.Time("temp_unschedulable_until").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),

		// temp_unschedulable_reason: 临时不可调度原因，便于排障审计
		field.String("temp_unschedulable_reason").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "text"}),

		// session_window_*: 会话窗口相关字段
		// 用于管理某些需要会话时间窗口的 API（如 Claude Pro）
		field.Time("session_window_start").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("session_window_end").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.String("session_window_status").
			Optional().
			Nillable().
			MaxLen(20),
	}
}

// Edges 定义账户实体的关联关系。
func (Account) Edges() []ent.Edge {
	return []ent.Edge{
		// groups: 账户所属的分组（多对多关系）
		// 通过 account_groups 中间表实现
		// 一个账户可以属于多个分组，一个分组可以包含多个账户
		edge.To("groups", Group.Type).
			Through("account_groups", AccountGroup.Type),
		// proxy: 账户使用的代理配置（可选的一对一关系）
		// 使用已有的 proxy_id 外键字段
		edge.To("proxy", Proxy.Type).
			Field("proxy_id").
			Unique(),
		// usage_logs: 该账户的使用日志
		edge.To("usage_logs", UsageLog.Type),
	}
}

// Indexes 定义数据库索引，优化查询性能。
// 每个索引对应一个常用的查询条件。
func (Account) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("platform"),            // 按平台筛选
		index.Fields("type"),                // 按认证类型筛选
		index.Fields("status"),              // 按状态筛选
		index.Fields("proxy_id"),            // 按代理筛选
		index.Fields("priority"),            // 按优先级排序
		index.Fields("last_used_at"),        // 按最后使用时间排序
		index.Fields("schedulable"),         // 筛选可调度账户
		index.Fields("rate_limited_at"),     // 筛选速率限制账户
		index.Fields("rate_limit_reset_at"), // 筛选速率限制解除时间
		index.Fields("overload_until"),      // 筛选过载账户
		// 调度热路径复合索引（线上由 SQL 迁移创建部分索引，schema 仅用于模型可读性对齐）
		index.Fields("platform", "priority"),
		index.Fields("priority", "status"),
		index.Fields("deleted_at"), // 软删除查询优化
	}
}
