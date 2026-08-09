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

// VideoPricing 视频模型分辨率定价表。
//
// 每一行代表 (model_slug × resolution) 组合下每秒的美元/元定价（price_per_second）。
// 计费公式：cost = price_per_second * duration_seconds * rate_multiplier。
//
// 定价缺失时视频任务提交将被拒绝（避免"零费用刷视频"）。
// 默认定价通过 startup migration 灌入（fal 官方价 × 1.1 作为兜底）。
type VideoPricing struct {
	ent.Schema
}

func (VideoPricing) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "video_pricings"},
	}
}

func (VideoPricing) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
	}
}

func (VideoPricing) Fields() []ent.Field {
	return []ent.Field{
		field.String("model_slug").
			MaxLen(200).
			NotEmpty().
			Comment("fal 视频模型 slug（如 fal-ai/bytedance/seedance-2.5/text-to-video）"),
		field.String("resolution").
			MaxLen(16).
			NotEmpty().
			Comment("视频分辨率（480p/720p/1080p/4k），与请求参数中的 resolution 直接匹配"),
		field.Float("price_per_second").
			Default(0).
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,10)"}).
			Comment("每秒美元/元定价（外部货币单位与 usage_log 保持一致）"),
		field.String("currency").
			MaxLen(8).
			Default("USD").
			Comment("计价货币（默认 USD，实际按渠道/账户结算）"),
		field.Bool("enabled").
			Default(true).
			Comment("是否启用；禁用后该 (model,resolution) 定价视为缺失"),
		field.String("note").
			MaxLen(512).
			Optional().
			Comment("备注（例如引用来源、更新说明）"),
	}
}

func (VideoPricing) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("model_slug", "resolution").Unique(),
		index.Fields("model_slug"),
	}
}
