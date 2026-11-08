//go:build unit

package service

import (
	"context"
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/stretchr/testify/require"
)

func TestNormalizeRechargeBonusTiers(t *testing.T) {
	t.Run("sorts ascending by min amount", func(t *testing.T) {
		out, err := NormalizeRechargeBonusTiers([]RechargeBonusTier{
			{MinAmount: 1000, BonusPercent: 35},
			{MinAmount: 100, BonusPercent: 20},
			{MinAmount: 500, BonusPercent: 30},
		})
		require.NoError(t, err)
		require.Equal(t, []RechargeBonusTier{
			{MinAmount: 100, BonusPercent: 20},
			{MinAmount: 500, BonusPercent: 30},
			{MinAmount: 1000, BonusPercent: 35},
		}, out)
	})

	t.Run("empty input yields empty non-nil slice", func(t *testing.T) {
		out, err := NormalizeRechargeBonusTiers(nil)
		require.NoError(t, err)
		require.NotNil(t, out)
		require.Len(t, out, 0)
	})

	t.Run("allows zero threshold and zero percent", func(t *testing.T) {
		out, err := NormalizeRechargeBonusTiers([]RechargeBonusTier{{MinAmount: 0, BonusPercent: 0}})
		require.NoError(t, err)
		require.Len(t, out, 1)
	})

	t.Run("rejects invalid values", func(t *testing.T) {
		cases := map[string][]RechargeBonusTier{
			"negative min":        {{MinAmount: -1, BonusPercent: 10}},
			"min three decimals":  {{MinAmount: 100.123, BonusPercent: 10}},
			"negative percent":    {{MinAmount: 100, BonusPercent: -5}},
			"percent over limit":  {{MinAmount: 100, BonusPercent: 1000.01}},
			"percent 3 decimals":  {{MinAmount: 100, BonusPercent: 12.345}},
			"duplicate min":       {{MinAmount: 100, BonusPercent: 10}, {MinAmount: 100, BonusPercent: 20}},
			"duplicate min 2 dec": {{MinAmount: 100, BonusPercent: 10}, {MinAmount: 100.00, BonusPercent: 20}},
		}
		for name, tiers := range cases {
			_, err := NormalizeRechargeBonusTiers(tiers)
			require.Error(t, err, name)
		}
	})

	t.Run("rejects too many tiers", func(t *testing.T) {
		tiers := make([]RechargeBonusTier, 0, maxRechargeBonusTiers+1)
		for i := 0; i <= maxRechargeBonusTiers; i++ {
			tiers = append(tiers, RechargeBonusTier{MinAmount: float64(i + 1), BonusPercent: 1})
		}
		_, err := NormalizeRechargeBonusTiers(tiers)
		require.Error(t, err)
	})
}

func TestParseRechargeBonusTiers(t *testing.T) {
	t.Run("empty or invalid json yields empty slice", func(t *testing.T) {
		require.NotNil(t, parseRechargeBonusTiers(""))
		require.Len(t, parseRechargeBonusTiers(""), 0)
		require.Len(t, parseRechargeBonusTiers("not json"), 0)
	})

	t.Run("drops invalid entries keeps first duplicate and sorts", func(t *testing.T) {
		raw := `[{"min_amount":500,"bonus_percent":30},{"min_amount":-1,"bonus_percent":5},` +
			`{"min_amount":100,"bonus_percent":20},{"min_amount":100,"bonus_percent":99},` +
			`{"min_amount":50,"bonus_percent":5000}]`
		out := parseRechargeBonusTiers(raw)
		require.Equal(t, []RechargeBonusTier{
			{MinAmount: 100, BonusPercent: 20},
			{MinAmount: 500, BonusPercent: 30},
		}, out)
	})

	t.Run("round trips encode", func(t *testing.T) {
		tiers := []RechargeBonusTier{{MinAmount: 100, BonusPercent: 20}, {MinAmount: 500, BonusPercent: 30}}
		encoded, err := encodeRechargeBonusTiers(tiers)
		require.NoError(t, err)
		require.Equal(t, tiers, parseRechargeBonusTiers(encoded))

		empty, err := encodeRechargeBonusTiers(nil)
		require.NoError(t, err)
		require.Equal(t, "", empty)
	})
}

func TestMatchRechargeBonusTier(t *testing.T) {
	tiers := []RechargeBonusTier{
		{MinAmount: 100, BonusPercent: 20},
		{MinAmount: 500, BonusPercent: 30},
		{MinAmount: 1000, BonusPercent: 35},
	}
	cases := []struct {
		amount  float64
		percent float64
		ok      bool
	}{
		{amount: 0, ok: false},
		{amount: 99.99, ok: false},
		{amount: 100, percent: 20, ok: true},
		{amount: 499.99, percent: 20, ok: true},
		{amount: 500, percent: 30, ok: true},
		{amount: 1000, percent: 35, ok: true},
		{amount: 1000000, percent: 35, ok: true},
	}
	for _, tc := range cases {
		tier, ok := matchRechargeBonusTier(tiers, tc.amount)
		require.Equal(t, tc.ok, ok, "amount %v", tc.amount)
		if ok {
			require.Equal(t, tc.percent, tier.BonusPercent, "amount %v", tc.amount)
		}
	}

	t.Run("float boundary 0.1+0.2 still matches 0.3 threshold", func(t *testing.T) {
		_, ok := matchRechargeBonusTier([]RechargeBonusTier{{MinAmount: 0.3, BonusPercent: 1}}, 0.1+0.2)
		require.True(t, ok)
	})

	t.Run("no tiers never matches", func(t *testing.T) {
		_, ok := matchRechargeBonusTier(nil, 100)
		require.False(t, ok)
	})
}

func TestCalculateRechargeBonusRounding(t *testing.T) {
	require.Equal(t, 20.0, calculateRechargeBonus(100, 20))
	require.Equal(t, 120.0, addRechargeBonus(100, 20))
	// 33.33 * 15% = 4.9995 → 5.00
	require.Equal(t, 5.0, calculateRechargeBonus(33.33, 15))
	require.Zero(t, calculateRechargeBonus(100, 0))
	require.Zero(t, calculateRechargeBonus(0, 20))

	// 阈值按支付金额命中，赠送按到账基数计算：1000 CNY × 0.14 = 140 USD，命中 1000 档 30% → 42
	cfg := &PaymentConfig{
		BalanceRechargeMultiplier: 0.14,
		RechargeBonusTiers:        []RechargeBonusTier{{MinAmount: 100, BonusPercent: 20}, {MinAmount: 500, BonusPercent: 30}},
	}
	require.Equal(t, rechargeBonusQuote{PayBase: 1000, Credited: 182, Bonus: 42, Percent: 30}, quoteRechargeBonus(cfg, 1000, "CNY"))
	// 命中 0% 档位视为无优惠
	cfg.RechargeBonusTiers = []RechargeBonusTier{{MinAmount: 10, BonusPercent: 0}}
	require.Equal(t, rechargeBonusQuote{PayBase: 50, Credited: 7}, quoteRechargeBonus(cfg, 50, "CNY"))
}

func TestParsePaymentConfigRechargeBonus(t *testing.T) {
	svc := &PaymentConfigService{}

	t.Run("defaults", func(t *testing.T) {
		cfg := svc.parsePaymentConfig(map[string]string{})
		require.NotNil(t, cfg.RechargeBonusTiers)
		require.Len(t, cfg.RechargeBonusTiers, 0)
		require.Equal(t, "", cfg.RechargeBonusNotice)
	})

	t.Run("reads tiers and notice", func(t *testing.T) {
		cfg := svc.parsePaymentConfig(map[string]string{
			SettingRechargeBonusTiers:  `[{"min_amount":500,"bonus_percent":30},{"min_amount":100,"bonus_percent":20}]`,
			SettingRechargeBonusNotice: "**满 100 送 20%**",
		})
		require.Equal(t, []RechargeBonusTier{{MinAmount: 100, BonusPercent: 20}, {MinAmount: 500, BonusPercent: 30}}, cfg.RechargeBonusTiers)
		require.Equal(t, "**满 100 送 20%**", cfg.RechargeBonusNotice)
	})
}

func TestUpdatePaymentConfigRechargeBonus(t *testing.T) {
	ctx := context.Background()

	t.Run("persists normalized tiers and trimmed notice", func(t *testing.T) {
		repo := &paymentConfigSettingRepoStub{values: map[string]string{}}
		svc := &PaymentConfigService{settingRepo: repo}
		tiers := []RechargeBonusTier{{MinAmount: 500, BonusPercent: 30}, {MinAmount: 100, BonusPercent: 20}}
		notice := "  活动文案  "
		require.NoError(t, svc.UpdatePaymentConfig(ctx, UpdatePaymentConfigRequest{
			RechargeBonusTiers:  &tiers,
			RechargeBonusNotice: &notice,
		}))
		require.Equal(t, `[{"min_amount":100,"bonus_percent":20},{"min_amount":500,"bonus_percent":30}]`, repo.updates[SettingRechargeBonusTiers])
		require.Equal(t, "活动文案", repo.updates[SettingRechargeBonusNotice])

		cfg, err := svc.GetPaymentConfig(ctx)
		require.NoError(t, err)
		require.Equal(t, []RechargeBonusTier{{MinAmount: 100, BonusPercent: 20}, {MinAmount: 500, BonusPercent: 30}}, cfg.RechargeBonusTiers)
	})

	t.Run("empty tiers clears setting and omitted fields are untouched", func(t *testing.T) {
		repo := &paymentConfigSettingRepoStub{values: map[string]string{
			SettingRechargeBonusTiers:  `[{"min_amount":100,"bonus_percent":20}]`,
			SettingRechargeBonusNotice: "keep me",
		}}
		svc := &PaymentConfigService{settingRepo: repo}
		empty := []RechargeBonusTier{}
		require.NoError(t, svc.UpdatePaymentConfig(ctx, UpdatePaymentConfigRequest{RechargeBonusTiers: &empty}))
		value, ok := repo.updates[SettingRechargeBonusTiers]
		require.True(t, ok)
		require.Equal(t, "", value)
		_, touched := repo.updates[SettingRechargeBonusNotice]
		require.False(t, touched)
		require.Equal(t, "keep me", repo.values[SettingRechargeBonusNotice])
	})

	t.Run("rejects invalid tiers", func(t *testing.T) {
		repo := &paymentConfigSettingRepoStub{values: map[string]string{}}
		svc := &PaymentConfigService{settingRepo: repo}
		bad := []RechargeBonusTier{{MinAmount: 100, BonusPercent: 20}, {MinAmount: 100, BonusPercent: 30}}
		err := svc.UpdatePaymentConfig(ctx, UpdatePaymentConfigRequest{RechargeBonusTiers: &bad})
		require.Error(t, err)
		require.Nil(t, repo.updates)
	})
}

func TestAffiliateRebateBaseAmountExcludesRechargeBonus(t *testing.T) {
	require.Equal(t, 100.0, affiliateRebateBaseAmount(&dbent.PaymentOrder{
		OrderType: payment.OrderTypeBalance, Amount: 130, BonusAmount: 30,
	}))
	require.Equal(t, 130.0, affiliateRebateBaseAmount(&dbent.PaymentOrder{
		OrderType: payment.OrderTypeBalance, Amount: 130,
	}))
	// 订阅订单不受 bonus 字段影响
	require.Equal(t, 50.0, affiliateRebateBaseAmount(&dbent.PaymentOrder{
		OrderType: payment.OrderTypeSubscription, Amount: 50, BonusAmount: 30,
	}))
	// 异常数据：赠送大于总额时钳到 0
	require.Equal(t, 0.0, affiliateRebateBaseAmount(&dbent.PaymentOrder{
		OrderType: payment.OrderTypeBalance, Amount: 10, BonusAmount: 30,
	}))
}

func TestNormalizeRechargeBonusMode(t *testing.T) {
	for raw, want := range map[string]string{"": RechargeBonusModeBonus, "bonus": RechargeBonusModeBonus, " Discount ": RechargeBonusModeDiscount} {
		mode, ok := NormalizeRechargeBonusMode(raw)
		require.True(t, ok, raw)
		require.Equal(t, want, mode, raw)
	}
	mode, ok := NormalizeRechargeBonusMode("cashback")
	require.False(t, ok)
	require.Equal(t, RechargeBonusModeBonus, mode)
}

func TestValidateRechargeBonusTiersForMode(t *testing.T) {
	tiers := []RechargeBonusTier{{MinAmount: 100, BonusPercent: 20}, {MinAmount: 500, BonusPercent: 100}}
	require.NoError(t, ValidateRechargeBonusTiersForMode(RechargeBonusModeBonus, tiers))
	require.Error(t, ValidateRechargeBonusTiersForMode(RechargeBonusModeDiscount, tiers))
	require.NoError(t, ValidateRechargeBonusTiersForMode(RechargeBonusModeDiscount, tiers[:1]))
}

func TestQuoteRechargeBonus(t *testing.T) {
	tiers := []RechargeBonusTier{{MinAmount: 100, BonusPercent: 20}, {MinAmount: 500, BonusPercent: 50}}

	t.Run("bonus mode keeps pay base and inflates credit", func(t *testing.T) {
		cfg := &PaymentConfig{BalanceRechargeMultiplier: 1, RechargeBonusTiers: tiers, RechargeBonusMode: RechargeBonusModeBonus}
		q := quoteRechargeBonus(cfg, 100, "USD")
		require.Equal(t, rechargeBonusQuote{PayBase: 100, Credited: 120, Bonus: 20, Percent: 20}, q)

		// 未命中：无优惠
		q = quoteRechargeBonus(cfg, 50, "USD")
		require.Equal(t, rechargeBonusQuote{PayBase: 50, Credited: 50}, q)
	})

	t.Run("discount mode keeps credit and reduces pay base", func(t *testing.T) {
		cfg := &PaymentConfig{BalanceRechargeMultiplier: 1, RechargeBonusTiers: tiers, RechargeBonusMode: RechargeBonusModeDiscount}
		q := quoteRechargeBonus(cfg, 500, "USD")
		require.Equal(t, rechargeBonusQuote{PayBase: 250, Credited: 500, Bonus: 250, Percent: 50}, q)

		// 倍率 0.14：1000 CNY 到账 140 USD；20% off 实付 800 CNY，免费部分 = 140 − 112 = 28 USD
		cfg.BalanceRechargeMultiplier = 0.14
		q = quoteRechargeBonus(cfg, 1000, "CNY")
		require.Equal(t, rechargeBonusQuote{PayBase: 500, Credited: 140, Bonus: 70, Percent: 50}, q)
		q = quoteRechargeBonus(cfg, 200, "CNY")
		require.Equal(t, rechargeBonusQuote{PayBase: 160, Credited: 28, Bonus: 5.6, Percent: 20}, q)
	})

	t.Run("discount rounds pay base to currency precision", func(t *testing.T) {
		cfg := &PaymentConfig{BalanceRechargeMultiplier: 1, RechargeBonusTiers: []RechargeBonusTier{{MinAmount: 1, BonusPercent: 15}}, RechargeBonusMode: RechargeBonusModeDiscount}
		require.Equal(t, 85.85, quoteRechargeBonus(cfg, 101, "USD").PayBase)
		require.Equal(t, 86.0, quoteRechargeBonus(cfg, 101, "JPY").PayBase)
	})

	t.Run("discount percent at or above 100 is ignored fail-safe", func(t *testing.T) {
		cfg := &PaymentConfig{BalanceRechargeMultiplier: 1, RechargeBonusTiers: []RechargeBonusTier{{MinAmount: 1, BonusPercent: 100}}, RechargeBonusMode: RechargeBonusModeDiscount}
		require.Equal(t, rechargeBonusQuote{PayBase: 100, Credited: 100}, quoteRechargeBonus(cfg, 100, "USD"))
	})

	t.Run("nil config and empty tiers yield plain conversion", func(t *testing.T) {
		require.Equal(t, rechargeBonusQuote{PayBase: 100, Credited: 100}, quoteRechargeBonus(nil, 100, "USD"))
		require.Equal(t, rechargeBonusQuote{PayBase: 100, Credited: 14}, quoteRechargeBonus(&PaymentConfig{BalanceRechargeMultiplier: 0.14}, 100, "CNY"))
	})
}

func TestParsePaymentConfigRechargeBonusMode(t *testing.T) {
	svc := &PaymentConfigService{}
	require.Equal(t, RechargeBonusModeBonus, svc.parsePaymentConfig(map[string]string{}).RechargeBonusMode)
	require.Equal(t, RechargeBonusModeDiscount, svc.parsePaymentConfig(map[string]string{SettingRechargeBonusMode: "discount"}).RechargeBonusMode)
	require.Equal(t, RechargeBonusModeBonus, svc.parsePaymentConfig(map[string]string{SettingRechargeBonusMode: "junk"}).RechargeBonusMode)
}

func TestUpdatePaymentConfigRechargeBonusMode(t *testing.T) {
	ctx := context.Background()

	t.Run("persists discount mode with valid tiers", func(t *testing.T) {
		repo := &paymentConfigSettingRepoStub{values: map[string]string{}}
		svc := &PaymentConfigService{settingRepo: repo}
		mode := "discount"
		tiers := []RechargeBonusTier{{MinAmount: 100, BonusPercent: 20}}
		require.NoError(t, svc.UpdatePaymentConfig(ctx, UpdatePaymentConfigRequest{RechargeBonusTiers: &tiers, RechargeBonusMode: &mode}))
		require.Equal(t, "discount", repo.updates[SettingRechargeBonusMode])
		cfg, err := svc.GetPaymentConfig(ctx)
		require.NoError(t, err)
		require.Equal(t, RechargeBonusModeDiscount, cfg.RechargeBonusMode)
	})

	t.Run("rejects unknown mode", func(t *testing.T) {
		repo := &paymentConfigSettingRepoStub{values: map[string]string{}}
		svc := &PaymentConfigService{settingRepo: repo}
		mode := "cashback"
		require.Error(t, svc.UpdatePaymentConfig(ctx, UpdatePaymentConfigRequest{RechargeBonusMode: &mode}))
		require.Nil(t, repo.updates)
	})

	t.Run("switching to discount with stored tiers at 100 percent is rejected", func(t *testing.T) {
		repo := &paymentConfigSettingRepoStub{values: map[string]string{
			SettingRechargeBonusTiers: `[{"min_amount":100,"bonus_percent":100}]`,
		}}
		svc := &PaymentConfigService{settingRepo: repo}
		mode := "discount"
		require.Error(t, svc.UpdatePaymentConfig(ctx, UpdatePaymentConfigRequest{RechargeBonusMode: &mode}))
		require.Nil(t, repo.updates)
	})

	t.Run("saving tiers at 100 percent while stored mode is discount is rejected", func(t *testing.T) {
		repo := &paymentConfigSettingRepoStub{values: map[string]string{SettingRechargeBonusMode: "discount"}}
		svc := &PaymentConfigService{settingRepo: repo}
		tiers := []RechargeBonusTier{{MinAmount: 100, BonusPercent: 100}}
		require.Error(t, svc.UpdatePaymentConfig(ctx, UpdatePaymentConfigRequest{RechargeBonusTiers: &tiers}))
		require.Nil(t, repo.updates)
		// 同样的档位在赠金模式下合法
		repo.values[SettingRechargeBonusMode] = "bonus"
		require.NoError(t, svc.UpdatePaymentConfig(ctx, UpdatePaymentConfigRequest{RechargeBonusTiers: &tiers}))
	})
}
