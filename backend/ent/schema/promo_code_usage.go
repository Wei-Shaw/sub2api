package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// PromoCodeUsage holds the schema definition for the PromoCodeUsage entity.
//
// 优惠码使用记录：记录每个用户使用优惠码的情况
type PromoCodeUsage struct {
	ent.Schema
}

func (PromoCodeUsage) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "promo_code_usages"},
	}
}

func (PromoCodeUsage) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("promo_code_id").
			Comment("优惠码ID"),
		field.Int64("user_id").
			Comment("使用用户ID"),
		field.Float("bonus_amount").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}).
			Comment("实际赠送金额"),
		field.Time("used_at").
			Default(time.Now).
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}).
			Comment("使用时间"),
	}
}

func (PromoCodeUsage) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("promo_code", PromoCode.Type).
			Ref("usage_records").
			Field("promo_code_id").
			Required().
			Unique(),
		edge.From("user", User.Type).
			Ref("promo_code_usages").
			Field("user_id").
			Required().
			Unique(),
	}
}

func (PromoCodeUsage) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("promo_code_id"),
		index.Fields("user_id"),
		// 每个用户每个优惠码只能使用一次
		index.Fields("promo_code_id", "user_id").Unique(),
	}
}
