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

// AsyncMediaTask 异步媒体任务表（fal 等异步图片平台）。
//
// 与只追加的 usage_logs 不同，本表是可变的，承载任务的完整生命周期：
// 提交（pending）→ 运行（running）→ 成功（succeeded）/ 失败退费（failed→refunded）/ 超期（expired）。
// 上游 request_id、预扣/结算费用、出图地址与转存地址均落于此表，由后台 reconciler 兜底对账。
type AsyncMediaTask struct {
	ent.Schema
}

// Annotations 返回 schema 的注解配置。
func (AsyncMediaTask) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "async_media_tasks"},
	}
}

// Mixin 复用统一的 created_at/updated_at 时间戳。
func (AsyncMediaTask) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
	}
}

// Fields 定义异步媒体任务的所有字段。
func (AsyncMediaTask) Fields() []ent.Field {
	return []ent.Field{
		// 标识与上游关联
		field.String("internal_request_id").
			MaxLen(64).
			NotEmpty().
			Comment("网关内部请求 ID（对外暴露给客户端轮询）"),
		field.String("upstream_request_id").
			MaxLen(128).
			Optional().
			Nillable().
			Comment("上游 fal request_id（提交成功后回填）"),
		field.String("status_url").
			MaxLen(512).
			Optional().
			Nillable().
			Comment("上游 status_url（以提交响应为准，避免手拼路径）"),
		field.String("response_url").
			MaxLen(512).
			Optional().
			Nillable().
			Comment("上游 response_url（取结果地址）"),

		// 归属维度
		field.Int64("account_id").
			Optional().
			Nillable().
			Comment("选中的上游账号 ID"),
		field.Int64("api_key_id").
			Comment("发起请求的 API Key ID"),
		field.Int64("user_id").
			Comment("所属用户 ID"),
		field.Int64("group_id").
			Optional().
			Nillable().
			Comment("所属分组 ID"),
		field.Int64("channel_id").
			Optional().
			Nillable().
			Comment("命中的渠道 ID"),

		// 门面与模型
		field.String("facade").
			MaxLen(16).
			Default("openai").
			Comment("对外门面协议：openai（伪同步）/ fal（原生异步）"),
		field.String("requested_model").
			MaxLen(100).
			Comment("客户端请求的模型名"),
		field.String("upstream_model").
			MaxLen(100).
			Optional().
			Nillable().
			Comment("映射后的上游模型/slug"),

		// 图片参数
		field.String("image_size").
			MaxLen(32).
			Optional().
			Nillable().
			Comment("请求的图片尺寸（OpenAI 形式或 fal 枚举）"),
		field.String("quality").
			MaxLen(16).
			Optional().
			Nillable().
			Comment("图片质量（auto/low/medium/high 等）"),
		field.Int("num_images").
			Default(1).
			Comment("请求生成的图片数量"),

		// 状态与计费
		field.String("status").
			MaxLen(16).
			Default("pending").
			Comment("任务状态：pending/running/succeeded/failed/refunded/expired"),
		field.Float("held_cost").
			Default(0).
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,10)"}).
			Comment("预扣费金额"),
		field.Float("final_cost").
			Default(0).
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,10)"}).
			Comment("结算费用（成功后写入；退费置 0）"),
		field.Float("rate_multiplier").
			Default(1).
			SchemaType(map[string]string{dialect.Postgres: "decimal(10,4)"}).
			Comment("计费倍率快照"),
		field.String("size_tier").
			MaxLen(16).
			Optional().
			Nillable().
			Comment("计费用尺寸档位快照（1K/2K/4K 等）"),

		// 结果与转存
		field.JSON("image_urls", []string{}).
			Optional().
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}).
			Comment("成功出图的 fal 原始 url 列表"),
		field.JSON("cos_urls", []string{}).
			Optional().
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}).
			Comment("转存 COS 后的 url 列表（转存失败则留空）"),

		// 错误与超时
		field.String("error_reason").
			MaxLen(512).
			Optional().
			Nillable().
			Comment("失败/退费原因"),
		field.Time("fail_deadline_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}).
			Comment("失败兜底截止时间（到达仍未完成则判定超期退费）"),
		field.Time("finished_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}).
			Comment("任务终结时间"),

		// 请求元信息（提交时持久化，供终态 usage_log 回填端点/IP/UA）。
		// 终态日志可能由后台 reconciler 或 fal 原生轮询写入，已脱离原始请求上下文，故落库于此。
		field.String("client_ip").
			MaxLen(45). // 支持 IPv6
			Optional().
			Nillable().
			Comment("客户端 IP"),
		field.String("user_agent").
			MaxLen(512).
			Optional().
			Nillable().
			Comment("客户端 User-Agent"),
		field.String("inbound_endpoint").
			MaxLen(100).
			Optional().
			Nillable().
			Comment("对外门面端点（客户端可见路径）"),
		field.String("upstream_endpoint").
			MaxLen(200).
			Optional().
			Nillable().
			Comment("上游 fal 端点（提交所用 slug 路径）"),
	}
}

// Indexes 定义数据库索引，优化扫描与查询。
func (AsyncMediaTask) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("internal_request_id").Unique(),
		index.Fields("upstream_request_id"),
		index.Fields("user_id"),
		index.Fields("api_key_id"),
		index.Fields("account_id"),
		index.Fields("status"),
		// reconciler 扫描未终结任务：status + fail_deadline_at
		index.Fields("status", "fail_deadline_at"),
		index.Fields("created_at"),
	}
}
