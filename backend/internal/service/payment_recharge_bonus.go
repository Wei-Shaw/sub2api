package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"strings"
	"unicode/utf8"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/shopspring/decimal"
)

// 充值优惠阶梯：余额充值订单按用户输入的支付金额命中阶梯（取不超过该金额的最大 MinAmount），
// 整条阶梯只有一种模式（RECHARGE_BONUS_MODE）：
//   - bonus（赠金）：实付不变，在到账基数（输入 × 充值倍率）之上额外赠送 BonusPercent% 的 USD 余额；
//   - discount（折扣）：到账不变（输入 × 倍率），实付基数按 BonusPercent% 打折。
//
// 两种模式落库形态相同：amount 为到账总额，bonus_amount 为其中「免费」的 USD 部分，pay_amount 为实收。
// 订阅订单不参与。优惠在下单时按当时配置计算并落库，后续改配置不影响已建订单。
const (
	// SettingRechargeBonusTiers 存 JSON 数组（RechargeBonusTier 列表），空/缺失表示未启用优惠。
	SettingRechargeBonusTiers = "RECHARGE_BONUS_TIERS"
	// SettingRechargeBonusNotice 充值页金额卡顶部展示的 Markdown 活动文案，空表示不展示。
	SettingRechargeBonusNotice = "RECHARGE_BONUS_NOTICE"
	// SettingRechargeBonusMode 阶梯模式：bonus / discount；空/非法按 bonus 解析（兼容早期配置）。
	SettingRechargeBonusMode = "RECHARGE_BONUS_MODE"
)

const (
	RechargeBonusModeBonus    = "bonus"
	RechargeBonusModeDiscount = "discount"
)

const (
	maxRechargeBonusTiers       = 20
	maxRechargeBonusPercent     = 1000
	maxRechargeBonusNoticeRunes = 10000
	rechargeBonusAmountEpsilon  = 1e-9
)

// RechargeBonusTier 一个优惠档位：支付金额 ≥ MinAmount 时按 BonusPercent% 赠送（bonus）或打折（discount）。
type RechargeBonusTier struct {
	MinAmount    float64 `json:"min_amount"`
	BonusPercent float64 `json:"bonus_percent"`
}

// NormalizeRechargeBonusMode 归一化模式；空按 bonus。第二个返回值表示输入是否合法。
func NormalizeRechargeBonusMode(raw string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", RechargeBonusModeBonus:
		return RechargeBonusModeBonus, true
	case RechargeBonusModeDiscount:
		return RechargeBonusModeDiscount, true
	default:
		return RechargeBonusModeBonus, false
	}
}

// ValidateRechargeBonusTiersForMode 折扣模式下百分比必须 < 100，否则实付为 0 或负数。
func ValidateRechargeBonusTiersForMode(mode string, tiers []RechargeBonusTier) error {
	if mode != RechargeBonusModeDiscount {
		return nil
	}
	for _, tier := range tiers {
		if tier.BonusPercent >= 100 {
			return fmt.Errorf("discount percent must be less than 100 (tier with min amount %s)",
				decimal.NewFromFloat(tier.MinAmount).Round(2).String())
		}
	}
	return nil
}

func rechargeBonusValueValid(v float64, max float64) bool {
	if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 || v > max {
		return false
	}
	d := decimal.NewFromFloat(v)
	return d.Equal(d.Round(2))
}

// NormalizeRechargeBonusTiers 严格归一化（写路径）：任何非法项直接报错；
// 成功时返回按 MinAmount 升序排序的副本。
func NormalizeRechargeBonusTiers(raw []RechargeBonusTier) ([]RechargeBonusTier, error) {
	if len(raw) == 0 {
		return []RechargeBonusTier{}, nil
	}
	if len(raw) > maxRechargeBonusTiers {
		return nil, fmt.Errorf("recharge bonus tiers exceed limit of %d", maxRechargeBonusTiers)
	}
	out := make([]RechargeBonusTier, 0, len(raw))
	seen := make(map[string]struct{}, len(raw))
	for _, tier := range raw {
		if !rechargeBonusValueValid(tier.MinAmount, math.MaxFloat64) {
			return nil, fmt.Errorf("recharge bonus tier min amount must be a non-negative number with at most 2 decimals")
		}
		if !rechargeBonusValueValid(tier.BonusPercent, maxRechargeBonusPercent) {
			return nil, fmt.Errorf("recharge bonus tier percent must be between 0 and %d with at most 2 decimals", maxRechargeBonusPercent)
		}
		key := decimal.NewFromFloat(tier.MinAmount).Round(2).String()
		if _, dup := seen[key]; dup {
			return nil, fmt.Errorf("duplicate recharge bonus tier min amount: %s", key)
		}
		seen[key] = struct{}{}
		out = append(out, RechargeBonusTier{MinAmount: tier.MinAmount, BonusPercent: tier.BonusPercent})
	}
	sortRechargeBonusTiers(out)
	return out, nil
}

func sortRechargeBonusTiers(tiers []RechargeBonusTier) {
	sort.SliceStable(tiers, func(i, j int) bool {
		return tiers[i].MinAmount < tiers[j].MinAmount
	})
}

// encodeRechargeBonusTiers 序列化为设置值；空列表存空串，与「未配置」保持同一形态。
func encodeRechargeBonusTiers(tiers []RechargeBonusTier) (string, error) {
	if len(tiers) == 0 {
		return "", nil
	}
	raw, err := json.Marshal(tiers)
	if err != nil {
		return "", fmt.Errorf("marshal recharge bonus tiers: %w", err)
	}
	return string(raw), nil
}

// parseRechargeBonusTiers 宽松解析（读路径）：非法条目丢弃而非报错，避免历史错配置阻断下单。
// 同一 MinAmount 重复时保留先出现的档位。始终返回非 nil 切片，便于 JSON 输出为 []。
func parseRechargeBonusTiers(raw string) []RechargeBonusTier {
	out := make([]RechargeBonusTier, 0)
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return out
	}
	var items []RechargeBonusTier
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		slog.Warn("[Payment] parseRechargeBonusTiers: unmarshal failed", "error", err)
		return out
	}
	seen := make(map[string]struct{}, len(items))
	for _, tier := range items {
		if !rechargeBonusValueValid(tier.MinAmount, math.MaxFloat64) || !rechargeBonusValueValid(tier.BonusPercent, maxRechargeBonusPercent) {
			continue
		}
		key := decimal.NewFromFloat(tier.MinAmount).Round(2).String()
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, tier)
	}
	sortRechargeBonusTiers(out)
	return out
}

func validateRechargeBonusNotice(notice string) error {
	if utf8.RuneCountInString(notice) > maxRechargeBonusNoticeRunes {
		return fmt.Errorf("recharge bonus notice exceeds %d characters", maxRechargeBonusNoticeRunes)
	}
	return nil
}

// resolveRechargeBonusUpdate 校验并归一化阶梯/模式更新。任一字段缺省时读取现值做交叉校验
// （折扣模式下所有档位百分比必须 < 100）。返回值仅在对应请求字段非 nil 时有意义。
func (s *PaymentConfigService) resolveRechargeBonusUpdate(ctx context.Context, req UpdatePaymentConfigRequest) (tiersValue string, modeValue string, err error) {
	if req.RechargeBonusTiers == nil && req.RechargeBonusMode == nil {
		return "", "", nil
	}
	stored := map[string]string{}
	if (req.RechargeBonusTiers == nil || req.RechargeBonusMode == nil) && s != nil && s.settingRepo != nil {
		stored, err = s.settingRepo.GetMultiple(ctx, []string{SettingRechargeBonusTiers, SettingRechargeBonusMode})
		if err != nil {
			return "", "", fmt.Errorf("get recharge bonus settings: %w", err)
		}
	}

	var tiers []RechargeBonusTier
	if req.RechargeBonusTiers != nil {
		tiers, err = NormalizeRechargeBonusTiers(*req.RechargeBonusTiers)
		if err != nil {
			return "", "", infraerrors.BadRequest("INVALID_RECHARGE_BONUS_TIERS", err.Error())
		}
	} else {
		tiers = parseRechargeBonusTiers(stored[SettingRechargeBonusTiers])
	}

	var mode string
	if req.RechargeBonusMode != nil {
		normalized, ok := NormalizeRechargeBonusMode(*req.RechargeBonusMode)
		if !ok {
			return "", "", infraerrors.BadRequest("INVALID_RECHARGE_BONUS_MODE", "recharge bonus mode must be bonus or discount")
		}
		mode = normalized
	} else {
		mode, _ = NormalizeRechargeBonusMode(stored[SettingRechargeBonusMode])
	}

	if err := ValidateRechargeBonusTiersForMode(mode, tiers); err != nil {
		return "", "", infraerrors.BadRequest("INVALID_RECHARGE_BONUS_TIERS", err.Error())
	}
	tiersValue, err = encodeRechargeBonusTiers(tiers)
	if err != nil {
		return "", "", err
	}
	return tiersValue, mode, nil
}

// matchRechargeBonusTier 返回不超过 paymentAmount 的最大档位；tiers 需已按 MinAmount 升序。
func matchRechargeBonusTier(tiers []RechargeBonusTier, paymentAmount float64) (RechargeBonusTier, bool) {
	if math.IsNaN(paymentAmount) || math.IsInf(paymentAmount, 0) || paymentAmount <= 0 {
		return RechargeBonusTier{}, false
	}
	var matched RechargeBonusTier
	found := false
	for _, tier := range tiers {
		if paymentAmount+rechargeBonusAmountEpsilon < tier.MinAmount {
			break
		}
		matched = tier
		found = true
	}
	return matched, found
}

// calculateRechargeBonus 赠送金额 = 到账基数 × 百分比，保留两位小数（四舍五入）。
func calculateRechargeBonus(baseCredited, bonusPercent float64) float64 {
	if baseCredited <= 0 || bonusPercent <= 0 || math.IsNaN(baseCredited) || math.IsNaN(bonusPercent) {
		return 0
	}
	return decimal.NewFromFloat(baseCredited).
		Mul(decimal.NewFromFloat(bonusPercent)).
		Div(decimal.NewFromInt(100)).
		Round(2).
		InexactFloat64()
}

// addRechargeBonus 到账总额 = 基数 + 赠送，两位小数。
func addRechargeBonus(baseCredited, bonus float64) float64 {
	return decimal.NewFromFloat(baseCredited).
		Add(decimal.NewFromFloat(bonus)).
		Round(2).
		InexactFloat64()
}

// calculateDiscountedPayBase 折扣模式实付基数 = 支付金额 × (1 − 百分比)，按币种精度四舍五入。
func calculateDiscountedPayBase(paymentAmount, discountPercent float64, currency string) float64 {
	digits := int32(payment.CurrencyMaxFractionDigits(currency))
	return decimal.NewFromFloat(paymentAmount).
		Mul(decimal.NewFromInt(100).Sub(decimal.NewFromFloat(discountPercent))).
		Div(decimal.NewFromInt(100)).
		Round(digits).
		InexactFloat64()
}

// rechargeBonusQuote 一笔余额充值的报价结果。
type rechargeBonusQuote struct {
	// PayBase 网关收款基数（支付币种，不含手续费）；赠金模式等于支付金额，折扣模式为折后金额。
	PayBase float64
	// Credited 到账总额（USD），含 Bonus。
	Credited float64
	// Bonus 免费额度（USD）：赠金模式为额外赠送，折扣模式为未付费却到账的部分。
	Bonus float64
	// Percent 命中档位的百分比；未命中或未产生优惠时为 0。
	Percent float64
}

// quoteRechargeBonus 按配置模式报价。currency 用于折扣模式实付基数的精度。
// 未配置阶梯、未命中、或折扣百分比 ≥ 100（非法历史数据，fail-safe）时按无优惠处理。
func quoteRechargeBonus(cfg *PaymentConfig, paymentAmount float64, currency string) rechargeBonusQuote {
	multiplier := defaultBalanceRechargeMultiplier
	var tiers []RechargeBonusTier
	mode := RechargeBonusModeBonus
	if cfg != nil {
		multiplier = cfg.BalanceRechargeMultiplier
		tiers = cfg.RechargeBonusTiers
		mode, _ = NormalizeRechargeBonusMode(cfg.RechargeBonusMode)
	}
	base := calculateCreditedBalance(paymentAmount, multiplier)
	quote := rechargeBonusQuote{PayBase: paymentAmount, Credited: base}

	tier, ok := matchRechargeBonusTier(tiers, paymentAmount)
	if !ok || tier.BonusPercent <= 0 {
		return quote
	}
	switch mode {
	case RechargeBonusModeDiscount:
		if tier.BonusPercent >= 100 {
			return quote
		}
		payBase := calculateDiscountedPayBase(paymentAmount, tier.BonusPercent, currency)
		if payBase <= 0 || payBase >= paymentAmount {
			return quote
		}
		paidCredit := calculateCreditedBalance(payBase, multiplier)
		bonus := decimal.NewFromFloat(base).Sub(decimal.NewFromFloat(paidCredit)).Round(2).InexactFloat64()
		if bonus < 0 {
			bonus = 0
		}
		quote.PayBase = payBase
		quote.Bonus = bonus
		quote.Percent = tier.BonusPercent
	default:
		bonus := calculateRechargeBonus(base, tier.BonusPercent)
		if bonus <= 0 {
			return quote
		}
		quote.Bonus = bonus
		quote.Credited = addRechargeBonus(base, bonus)
		quote.Percent = tier.BonusPercent
	}
	return quote
}

// paymentOrderAmountWithoutBonus 订单到账金额剔除免费额度后的实付部分（USD），用于推广返利基数。
func paymentOrderAmountWithoutBonus(o *dbent.PaymentOrder) float64 {
	if o == nil {
		return 0
	}
	if o.OrderType != payment.OrderTypeBalance || o.BonusAmount <= 0 {
		return o.Amount
	}
	base := decimal.NewFromFloat(o.Amount).
		Sub(decimal.NewFromFloat(o.BonusAmount)).
		Round(2).
		InexactFloat64()
	if base < 0 {
		return 0
	}
	return base
}
