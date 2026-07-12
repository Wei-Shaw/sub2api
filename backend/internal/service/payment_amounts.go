package service

import (
	"math"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/shopspring/decimal"
)

const defaultBalanceRechargeMultiplier = 1.0

func normalizeBalanceRechargeMultiplier(multiplier float64) float64 {
	if math.IsNaN(multiplier) || math.IsInf(multiplier, 0) || multiplier <= 0 {
		return defaultBalanceRechargeMultiplier
	}
	return multiplier
}

// normalizeSubscriptionUSDToCNYRate 将非法值归一为 0（换算关闭）。
// 与余额倍率不同，0 是合法状态：表示订阅保持 price 直付的存量行为。
func normalizeSubscriptionUSDToCNYRate(rate float64) float64 {
	if math.IsNaN(rate) || math.IsInf(rate, 0) || rate < 0 {
		return 0
	}
	return rate
}

func calculateCreditedBalance(paymentAmount, multiplier float64) float64 {
	return decimal.NewFromFloat(paymentAmount).
		Mul(decimal.NewFromFloat(normalizeBalanceRechargeMultiplier(multiplier))).
		Round(2).
		InexactFloat64()
}

// ceil2 向上取整到两位小数。
//
// 与 calculateCreditedBalance 的银行家舍入不同，bonus 使用向上取整：
//   - 一是匹配产品需求"赠送精确到两位小数向上取整"；
//   - 二是把舍入误差倾向用户，符合营销赠送的公平观感。
//
// 实现使用 shopspring/decimal 避免浮点漂移。
func ceil2(value float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0
	}
	return decimal.NewFromFloat(value).
		Mul(decimal.NewFromInt(100)).
		Ceil().
		Div(decimal.NewFromInt(100)).
		InexactFloat64()
}

// ResolveRechargeBonus 计算指定支付金额在当前活动下命中的赠送倍率与赠送金额。
//
// 返回 (rate, bonus)：
//   - rate: 命中档位的 BonusRate（未命中或未激活返回 0）。
//   - bonus: ceil2(payAmount × multiplier × rate)，单位与 credited_balance 一致。
//
// 关键约定：
//   - 档位匹配仍以 payAmount（用户实付）为准——tier.MinAmount 是支付金额阈值。
//   - 赠送基数是 credited_balance（即 pay_amount × balance_recharge_multiplier）；
//     倍率 ≤ 0 / NaN / Inf 时按 1.0 处理（normalizeBalanceRechargeMultiplier 兜底）。
//   - multiplier 走 decimal 与 calculateCreditedBalance 同源，避免浮点漂移。
//
// 当 promo 为 nil、Enabled = false、now 不在 [valid_from, valid_until] 区间内、
// 或 payAmount 低于最低档位时，均返回 (0, 0)。
//
// 该函数对 fulfillment 与前端预览（同算法 mirrors）都是单一事实来源。
func ResolveRechargeBonus(payAmount, multiplier float64, promo *RechargePromo, now time.Time) (rate float64, bonus float64) {
	if promo == nil || !promo.IsActiveAt(now) {
		return 0, 0
	}
	if math.IsNaN(payAmount) || math.IsInf(payAmount, 0) || payAmount <= 0 {
		return 0, 0
	}
	tier := promo.ResolveTier(payAmount)
	if tier == nil || tier.BonusRate <= 0 {
		return 0, 0
	}
	mult := normalizeBalanceRechargeMultiplier(multiplier)
	gross := decimal.NewFromFloat(payAmount).
		Mul(decimal.NewFromFloat(mult)).
		Mul(decimal.NewFromFloat(tier.BonusRate)).
		InexactFloat64()
	return tier.BonusRate, ceil2(gross)
}

func calculateGatewayRefundAmount(orderAmount, payAmount, refundAmount float64, currency string) float64 {
	if orderAmount <= 0 || payAmount <= 0 || refundAmount <= 0 {
		return 0
	}
	fractionDigits := int32(payment.CurrencyMaxFractionDigits(currency))
	if math.Abs(refundAmount-orderAmount) <= paymentAmountToleranceForCurrency(currency) {
		return decimal.NewFromFloat(payAmount).Round(fractionDigits).InexactFloat64()
	}
	return decimal.NewFromFloat(payAmount).
		Mul(decimal.NewFromFloat(refundAmount)).
		Div(decimal.NewFromFloat(orderAmount)).
		Round(fractionDigits).
		InexactFloat64()
}
