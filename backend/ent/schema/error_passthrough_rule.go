// Package schema 定义 Ent ORM 的数据库 schema。
package schema

import (
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// ErrorPassthroughRule 定义全局错误透传规则的 schema。
//
// 错误透传规则用于控制上游错误如何返回给客户端：
//   - 匹配条件：错误码 + 关键词组合
//   - 响应行为：透传原始信息 或 自定义错误信息
//   - 响应状态码：可指定返回给客户端的状态码
//   - 平台范围：规则适用的平台（Anthropic、OpenAI、Gemini、Antigravity）
type ErrorPassthroughRule struct {
	ent.Schema
}

// Annotations 返回 schema 的注解配置。
func (ErrorPassthroughRule) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "error_passthrough_rules"},
	}
}

// Mixin 返回该 schema 使用的混入组件。
func (ErrorPassthroughRule) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
	}
}

// Fields 定义错误透传规则实体的所有字段。
func (ErrorPassthroughRule) Fields() []ent.Field {
	return []ent.Field{
		// name: 规则名称，用于在界面中标识规则
		field.String("name").
			MaxLen(100).
			NotEmpty(),

		// enabled: 是否启用该规则
		field.Bool("enabled").
			Default(true),

		// priority: 规则优先级，数值越小优先级越高
		// 匹配时按优先级顺序检查，命中第一个匹配的规则
		field.Int("priority").
			Default(0),

		// error_codes: 匹配的错误码列表（OR关系）
		// 例如：[422, 400] 表示匹配 422 或 400 错误码
		field.JSON("error_codes", []int{}).
			Optional().
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}),

		// keywords: 匹配的关键词列表（OR关系）
		// 例如：["context limit", "model not supported"]
		// 关键词匹配不区分大小写
		field.JSON("keywords", []string{}).
			Optional().
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}),

		// match_mode: 匹配模式
		// - "any": 错误码匹配 OR 关键词匹配（任一条件满足即可）
		// - "all": 错误码匹配 AND 关键词匹配（所有条件都必须满足）
		field.String("match_mode").
			MaxLen(10).
			Default("any"),

		// platforms: 适用平台列表
		// 例如：["anthropic", "openai", "gemini", "antigravity"]
		// 空列表表示适用于所有平台
		field.JSON("platforms", []string{}).
			Optional().
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}),

		// passthrough_code: 是否透传上游原始状态码
		// true: 使用上游返回的状态码
		// false: 使用 response_code 指定的状态码
		field.Bool("passthrough_code").
			Default(true),

		// response_code: 自定义响应状态码
		// 当 passthrough_code=false 时使用此状态码
		field.Int("response_code").
			Optional().
			Nillable(),

		// passthrough_body: 是否透传上游原始错误信息
		// true: 使用上游返回的错误信息
		// false: 使用 custom_message 指定的错误信息
		field.Bool("passthrough_body").
			Default(true),

		// custom_message: 自定义错误信息
		// 当 passthrough_body=false 时使用此错误信息
		field.Text("custom_message").
			Optional().
			Nillable(),

		// skip_monitoring: 是否跳过运维监控记录
		// true: 匹配此规则的错误不会被记录到 ops_error_logs
		// false: 正常记录到运维监控（默认行为）
		field.Bool("skip_monitoring").
			Default(false),

		// description: 规则描述，用于说明规则的用途
		field.Text("description").
			Optional().
			Nillable(),
	}
}

// Indexes 定义数据库索引，优化查询性能。
func (ErrorPassthroughRule) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("enabled"),  // 筛选启用的规则
		index.Fields("priority"), // 按优先级排序
	}
}
