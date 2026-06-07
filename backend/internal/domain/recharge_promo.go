package domain

// RechargePromoTier 是充值赠送活动的一档配置。
//
// 之所以单独放在 domain 包：
//   - ent schema 的 field.JSON 需要传入一个具体类型作为值的"形状"。
//   - 直接在 ent/schema 包里定义会触发 ent 生成的 mutation.go 反向 import，
//     形成 schema → mixins → intercept → ent → schema 的循环。
//   - service / dto / handler 各层共用同一份内存结构，避免重复定义与转换。
//
// 字段约束（写入前由 service.validateRechargePromo 强校验）：
//   - MinAmount > 0；多档位时整体严格升序。
//   - BonusRate 落在 [0, 1)，1 = 100% 不允许（避免边界歧义）。
type RechargePromoTier struct {
	MinAmount float64 `json:"min_amount"`
	BonusRate float64 `json:"bonus_rate"`
}
