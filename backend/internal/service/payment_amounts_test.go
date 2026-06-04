package service

import (
	"testing"
	"time"
)

// TestResolveRechargeBonus 覆盖核心档位/边界/向上取整/活动开关/时间窗这些组合。
//
// 与前端 bonusForAmount 是镜像算法，必须保持行为一致。
func TestResolveRechargeBonus(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	pastFrom := now.Add(-1 * time.Hour)
	futureUntil := now.Add(1 * time.Hour)

	tiers := []RechargePromoTier{
		{MinAmount: 100, BonusRate: 0.05},
		{MinAmount: 500, BonusRate: 0.10},
		{MinAmount: 1000, BonusRate: 0.15},
	}

	activePromo := &RechargePromo{
		Enabled:    true,
		ValidFrom:  &pastFrom,
		ValidUntil: &futureUntil,
		Tiers:      tiers,
	}

	tests := []struct {
		name       string
		promo      *RechargePromo
		payAmount  float64
		multiplier float64
		now        time.Time
		wantRate   float64
		wantBonus  float64
	}{
		{
			name:       "nil promo returns zero",
			promo:      nil,
			payAmount:  1000,
			multiplier: 1,
			now:        now,
			wantRate:   0,
			wantBonus:  0,
		},
		{
			name: "disabled promo returns zero",
			promo: &RechargePromo{
				Enabled: false,
				Tiers:   tiers,
			},
			payAmount:  1000,
			multiplier: 1,
			now:        now,
			wantRate:   0,
			wantBonus:  0,
		},
		{
			name: "before valid_from returns zero",
			promo: &RechargePromo{
				Enabled:    true,
				ValidFrom:  &futureUntil, // start in the future relative to `now`
				ValidUntil: nil,
				Tiers:      tiers,
			},
			payAmount:  1000,
			multiplier: 1,
			now:        now,
			wantRate:   0,
			wantBonus:  0,
		},
		{
			name: "after valid_until returns zero",
			promo: &RechargePromo{
				Enabled:    true,
				ValidFrom:  nil,
				ValidUntil: &pastFrom, // ended already
				Tiers:      tiers,
			},
			payAmount:  1000,
			multiplier: 1,
			now:        now,
			wantRate:   0,
			wantBonus:  0,
		},
		{
			name:       "below lowest tier returns zero",
			promo:      activePromo,
			payAmount:  99.99,
			multiplier: 1,
			now:        now,
			wantRate:   0,
			wantBonus:  0,
		},
		{
			name:       "exactly at lowest tier hits 5%",
			promo:      activePromo,
			payAmount:  100,
			multiplier: 1,
			now:        now,
			wantRate:   0.05,
			wantBonus:  5,
		},
		{
			name:       "at 499 still in 5% tier",
			promo:      activePromo,
			payAmount:  499,
			multiplier: 1,
			now:        now,
			wantRate:   0.05,
			// 499 * 1 * 0.05 = 24.95 → ceil2 = 24.95
			wantBonus: 24.95,
		},
		{
			name:       "boundary 500 promotes to 10% tier",
			promo:      activePromo,
			payAmount:  500,
			multiplier: 1,
			now:        now,
			wantRate:   0.10,
			wantBonus:  50,
		},
		{
			name:       "highest tier wins at 1000",
			promo:      activePromo,
			payAmount:  1000,
			multiplier: 1,
			now:        now,
			wantRate:   0.15,
			wantBonus:  150,
		},
		{
			name:       "ceil2 rounds up fractional bonus",
			promo:      activePromo,
			payAmount:  100,
			multiplier: 1,
			now:        now,
			wantRate:   0.05,
			wantBonus:  5,
		},
		{
			name: "non-multiple of cent rounds up (0.0333 rate)",
			promo: &RechargePromo{
				Enabled:   true,
				ValidFrom: &pastFrom, ValidUntil: &futureUntil,
				Tiers: []RechargePromoTier{{MinAmount: 100, BonusRate: 0.0333}},
			},
			payAmount:  100,
			multiplier: 1,
			now:        now,
			wantRate:   0.0333,
			// 100 * 1 * 0.0333 = 3.33 → ceil2 = 3.33
			wantBonus: 3.33,
		},
		{
			name: "non-multiple of cent rounds up at 99 / 0.0333",
			promo: &RechargePromo{
				Enabled:   true,
				ValidFrom: &pastFrom, ValidUntil: &futureUntil,
				Tiers: []RechargePromoTier{{MinAmount: 50, BonusRate: 0.0333}},
			},
			payAmount:  99,
			multiplier: 1,
			now:        now,
			wantRate:   0.0333,
			// 99 * 1 * 0.0333 = 3.2967 → ceil2 = 3.30
			wantBonus: 3.30,
		},
		{
			name:       "negative pay amount returns zero",
			promo:      activePromo,
			payAmount:  -10,
			multiplier: 1,
			now:        now,
			wantRate:   0,
			wantBonus:  0,
		},
		{
			name:       "multiplier scales bonus base (CNY 100 × 0.14 × 5% → 0.70)",
			promo:      activePromo,
			payAmount:  100,
			multiplier: 0.14,
			now:        now,
			wantRate:   0.05,
			// 100 * 0.14 * 0.05 = 0.70 → ceil2 = 0.70
			wantBonus: 0.70,
		},
		{
			name:       "multiplier with ceil2 rounding (CNY 1000 × 0.14 × 15% → 21)",
			promo:      activePromo,
			payAmount:  1000,
			multiplier: 0.14,
			now:        now,
			wantRate:   0.15,
			// 1000 * 0.14 * 0.15 = 21.0 → ceil2 = 21.0
			wantBonus: 21,
		},
		{
			name: "multiplier triggers ceil2 round-up (333 × 0.14 × 5% = 2.331 → 2.34)",
			promo: &RechargePromo{
				Enabled:   true,
				ValidFrom: &pastFrom, ValidUntil: &futureUntil,
				Tiers: []RechargePromoTier{{MinAmount: 100, BonusRate: 0.05}},
			},
			payAmount:  333,
			multiplier: 0.14,
			now:        now,
			wantRate:   0.05,
			wantBonus:  2.34,
		},
		{
			name:       "non-positive multiplier falls back to 1.0",
			promo:      activePromo,
			payAmount:  100,
			multiplier: 0,
			now:        now,
			wantRate:   0.05,
			// fallback multiplier=1 → 100 * 1 * 0.05 = 5
			wantBonus: 5,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gotRate, gotBonus := ResolveRechargeBonus(tt.payAmount, tt.multiplier, tt.promo, tt.now)
			if gotRate != tt.wantRate {
				t.Errorf("rate = %v, want %v", gotRate, tt.wantRate)
			}
			if gotBonus != tt.wantBonus {
				t.Errorf("bonus = %v, want %v", gotBonus, tt.wantBonus)
			}
		})
	}
}

// TestRechargePromo_ResolveTier 覆盖 ResolveTier 的边界（升序 tiers、最高匹配档）。
func TestRechargePromo_ResolveTier(t *testing.T) {
	t.Parallel()
	promo := &RechargePromo{
		Tiers: []RechargePromoTier{
			{MinAmount: 100, BonusRate: 0.05},
			{MinAmount: 500, BonusRate: 0.10},
		},
	}

	if tier := promo.ResolveTier(50); tier != nil {
		t.Errorf("payAmount 50 should not match any tier, got %+v", tier)
	}
	if tier := promo.ResolveTier(100); tier == nil || tier.BonusRate != 0.05 {
		t.Errorf("payAmount 100 should match 5%% tier, got %+v", tier)
	}
	if tier := promo.ResolveTier(499); tier == nil || tier.BonusRate != 0.05 {
		t.Errorf("payAmount 499 should still be 5%% tier, got %+v", tier)
	}
	if tier := promo.ResolveTier(500); tier == nil || tier.BonusRate != 0.10 {
		t.Errorf("payAmount 500 should match 10%% tier, got %+v", tier)
	}
	if tier := promo.ResolveTier(1_000_000); tier == nil || tier.BonusRate != 0.10 {
		t.Errorf("very large amount should match highest tier, got %+v", tier)
	}
}
