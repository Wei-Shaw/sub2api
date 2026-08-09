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

// AsyncVideoTask 异步视频任务表（fal 视频异步平台，如 seedance 系列）。
//
// 与 async_media_task（图片）并行独立：
//   - 图片按 (size_tier × quality × num_images) 计费
//   - 视频按 (resolution × duration_seconds) 计费
//
// 生命周期：pending → running → succeeded / refunded / expired，
// 由伪同步阻塞或 reconciler 兜底推进。视频结果以 JSON 原样保存（fal 的 result 载荷），
// 通过 result_payload 字段透传给客户端。
type AsyncVideoTask struct {
	ent.Schema
}

func (AsyncVideoTask) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "async_video_tasks"},
	}
}

func (AsyncVideoTask) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
	}
}

func (AsyncVideoTask) Fields() []ent.Field {
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
			Comment("上游 status_url"),
		field.String("response_url").
			MaxLen(512).
			Optional().
			Nillable().
			Comment("上游 response_url"),

		// 归属维度
		field.Int64("account_id").Optional().Nillable().Comment("选中的上游账号 ID"),
		field.Int64("api_key_id").Comment("发起请求的 API Key ID"),
		field.Int64("user_id").Comment("所属用户 ID"),
		field.Int64("organization_id").Optional().Nillable(),
		field.Int64("payer_user_id").Optional().Nillable(),
		field.String("balance_source").MaxLen(16).Optional().Nillable(),
		field.Int64("authz_generation").Optional().Nillable(),
		field.Int64("group_id").Optional().Nillable().Comment("所属分组 ID"),
		field.Int64("channel_id").Optional().Nillable().Comment("命中的渠道 ID"),

		// 门面与模型
		field.String("facade").
			MaxLen(16).
			Default("fal").
			Comment("对外门面协议：fal（原生异步）"),
		field.String("requested_model").
			MaxLen(200).
			Comment("客户端请求的模型/slug"),
		field.String("upstream_model").
			MaxLen(200).
			Optional().
			Nillable().
			Comment("映射后的上游 fal slug"),

		// 视频参数
		field.String("resolution").
			MaxLen(16).
			Optional().
			Nillable().
			Comment("视频分辨率（480p/720p/1080p/4k），用于计费维度命中"),
		field.Int("duration_seconds").
			Default(0).
			Comment("视频时长（秒），用于计费"),
		field.String("aspect_ratio").
			MaxLen(16).
			Optional().
			Nillable().
			Comment("视频宽高比（16:9/9:16 等，仅记录）"),

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
		field.Float("unit_price_snapshot").
			Default(0).
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,10)"}).
			Comment("提交时的 price_per_second 快照"),

		// 请求 payload 与结果（原样透传）
		field.JSON("request_payload", map[string]any{}).
			Optional().
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}).
			Comment("客户端提交的 fal 请求 body（原样）"),
		field.JSON("result_payload", map[string]any{}).
			Optional().
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}).
			Comment("fal 上游 result 响应体（原样透传给客户端）"),
		field.JSON("video_urls", []string{}).
			Optional().
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}).
			Comment("从 result_payload 提取的视频 URL 列表（便于列表展示与转存）"),
		field.JSON("cos_urls", []string{}).
			Optional().
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}).
			Comment("转存 COS 后的 url 列表（当前预留，不启用转存）"),

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
			Comment("失败兜底截止时间"),
		field.Time("finished_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}).
			Comment("任务终结时间"),

		// 请求元信息
		field.String("client_ip").MaxLen(45).Optional().Nillable(),
		field.String("user_agent").MaxLen(512).Optional().Nillable(),
		field.String("inbound_endpoint").MaxLen(200).Optional().Nillable(),
		field.String("upstream_endpoint").MaxLen(200).Optional().Nillable(),
	}
}

func (AsyncVideoTask) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("internal_request_id").Unique(),
		index.Fields("upstream_request_id"),
		index.Fields("user_id"),
		index.Fields("organization_id", "created_at"),
		index.Fields("api_key_id"),
		index.Fields("account_id"),
		index.Fields("status"),
		index.Fields("status", "fail_deadline_at"),
		index.Fields("created_at"),
	}
}
