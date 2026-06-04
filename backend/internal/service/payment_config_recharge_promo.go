package service

import (
	"fmt"
	"math"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// RechargePromo describes the active recharge bonus campaign.
//
// 概念区分（重要）：
//   - BALANCE_RECHARGE_MULTIPLIER 是 CNY → USD 的永久倍率（影响 credited_balance），
//     与 RechargePromo 是完全独立的两件事。
//   - RechargePromo 只决定**额外赠送**到余额的 bonus_amount；与倍率是“加法”叠加。
//
// 文档/字段说明请见 openspec/changes/recharge-bonus-promo/proposal.md。
type RechargePromo struct {
	Enabled    bool                `json:"enabled"`
	ValidFrom  *time.Time          `json:"valid_from,omitempty"`
	ValidUntil *time.Time          `json:"valid_until,omitempty"`
	Tiers      []RechargePromoTier `json:"tiers"`
	// Version 由后端在保存时基于活动表 ID 生成（不透明字符串）。
	// 客户端用它作为 localStorage 红点 dismiss key 的一部分；
	// 对前端是不透明的，不需要关心其内部含义。
	Version string `json:"version,omitempty"`
	// ActivityID 是当前活动在 recharge_promo_activities 表中的自增 ID。
	// 仅当 RechargePromo 由 ActivityToPromo 构造（即来自数据库）时才有值，
	// 入站请求 / serializeRechargePromoSetting 等场景都视作零值。
	// fulfillment 用它把命中赠送的具体活动 id 落到 payment_orders.activity_id。
	ActivityID int64 `json:"activity_id,omitempty"`
}

// RechargePromoTier 一个赠送档位。
//   - MinAmount: 触发该档位的最低支付金额（含等于）。
//   - BonusRate: 赠送倍率，区间 [0, 1)。
type RechargePromoTier struct {
	MinAmount float64 `json:"min_amount"`
	BonusRate float64 `json:"bonus_rate"`
}

// IsActiveAt reports whether the promo is enabled and the supplied moment falls
// inside its validity window.
func (p *RechargePromo) IsActiveAt(now time.Time) bool {
	if p == nil || !p.Enabled {
		return false
	}
	if p.ValidFrom != nil && now.Before(*p.ValidFrom) {
		return false
	}
	if p.ValidUntil != nil && !now.Before(*p.ValidUntil) {
		// 包含 ValidUntil 时刻？需求里写"inclusive of the timestamp, exclusive of the next millisecond"。
		// 用 !Before 等价于 >=，已超出窗口
		return false
	}
	return true
}

// ResolveTier picks the highest tier whose MinAmount ≤ payAmount. Returns nil if
// no tier matches.
func (p *RechargePromo) ResolveTier(payAmount float64) *RechargePromoTier {
	if p == nil || len(p.Tiers) == 0 {
		return nil
	}
	var matched *RechargePromoTier
	for i := range p.Tiers {
		t := &p.Tiers[i]
		if payAmount+1e-9 >= t.MinAmount {
			matched = t
		} else {
			break
		}
	}
	return matched
}

// validateRechargePromo enforces the invariants documented in the spec.
// Returns a typed BadRequest error on the first violation.
func validateRechargePromo(p *RechargePromo) error {
	if p == nil || !p.Enabled {
		return nil
	}
	if len(p.Tiers) == 0 {
		return infraerrors.BadRequest("INVALID_RECHARGE_PROMO", "tiers must contain at least one entry when enabled")
	}
	if p.ValidFrom != nil && p.ValidUntil != nil && !p.ValidFrom.Before(*p.ValidUntil) {
		return infraerrors.BadRequest("INVALID_RECHARGE_PROMO", "valid_from must be earlier than valid_until")
	}
	var prev float64 = -1
	for i, t := range p.Tiers {
		if math.IsNaN(t.MinAmount) || math.IsInf(t.MinAmount, 0) || t.MinAmount <= 0 {
			return infraerrors.BadRequest("INVALID_RECHARGE_PROMO",
				fmt.Sprintf("tier %d: min_amount must be greater than 0", i))
		}
		if t.MinAmount <= prev {
			return infraerrors.BadRequest("INVALID_RECHARGE_PROMO", "tiers must be ascending by min_amount")
		}
		if math.IsNaN(t.BonusRate) || math.IsInf(t.BonusRate, 0) || t.BonusRate < 0 || t.BonusRate >= 1 {
			return infraerrors.BadRequest("INVALID_RECHARGE_PROMO",
				fmt.Sprintf("tier %d: bonus_rate must be in [0, 1)", i))
		}
		prev = t.MinAmount
	}
	return nil
}
