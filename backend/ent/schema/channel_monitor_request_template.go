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

// ChannelMonitorRequestTemplate 请求模板：一组可复用的 headers + 可选 body 覆盖配置。
//
// 语义为快照：模板被"应用"到监控时，extra_headers / body_override_mode / body_override
// 会被**拷贝**到 channel_monitors 同名字段；后续模板变动不会自动影响已应用的监控——
// 必须用户主动在模板编辑 Dialog 里点「应用到关联监控」才会覆盖快照。
// 这样模板改错不会瞬间打挂所有已经跑起来的监控。
type ChannelMonitorRequestTemplate struct {
	ent.Schema
}

func (ChannelMonitorRequestTemplate) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "channel_monitor_request_templates"},
	}
}

func (ChannelMonitorRequestTemplate) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
	}
}

func (ChannelMonitorRequestTemplate) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").
			NotEmpty().
			MaxLen(100),
		field.Enum("provider").
			Values("openai", "anthropic", "gemini"),
		field.String("description").
			Optional().
			Default("").
			MaxLen(500),
		// extra_headers: 用户自定义 HTTP 头（如 User-Agent 伪装）。
		// 运行时 merge 进 adapter 默认 headers，用户值优先；
		// hop-by-hop 黑名单（Host/Content-Length/...）由 checker 过滤。
		field.JSON("extra_headers", map[string]string{}).
			Default(map[string]string{}),
		// body_override_mode: 'off' | 'merge' | 'replace'
		//   off     - 用 adapter 默认 body（忽略 body_override）
		//   merge   - adapter 默认 body 与 body_override 浅合并（body_override 优先，
		//             model/messages/contents 等关键字段在 checker 里走黑名单跳过）
		//   replace - 直接用 body_override 作为完整 body；此时跳过 challenge 校验，
		//             改为 HTTP 2xx + 响应文本非空即视为可用
		field.String("body_override_mode").
			Default("off").
			MaxLen(10),
		// body_override: JSON 对象，根据 body_override_mode 使用。
		// 用 map[string]any 以便前端传任意结构（含嵌套）。
		field.JSON("body_override", map[string]any{}).
			Optional(),
	}
}

func (ChannelMonitorRequestTemplate) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("monitors", ChannelMonitor.Type).
			Ref("request_template"),
	}
}

func (ChannelMonitorRequestTemplate) Indexes() []ent.Index {
	return []ent.Index{
		// 同一 provider 内 name 唯一：允许 Anthropic + OpenAI 重名 "伪装官方客户端"。
		index.Fields("provider", "name").Unique(),
	}
}
