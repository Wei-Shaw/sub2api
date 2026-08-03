package service

import (
	"math"
	"strings"

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

// === v4.6.2 currency separation helpers (owner spec 2026-08-02) ===

// NormalizePaymentCurrency 带默认值的币种归一化（空值/非法值返 defaultVal）。
// 与 payment.NormalizePaymentCurrency 不同：后者空值返 "CNY" 硬编码。
func NormalizePaymentCurrency(raw, defaultVal string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return defaultVal
	}
	v, err := payment.NormalizePaymentCurrency(raw)
	if err != nil || v == "" {
		return defaultVal
	}
	return v
}

// NormalizeFXFallbackRate 兜底汇率归一化（非法值返 0，调用方需再赋默认值）。
func NormalizeFXFallbackRate(rate float64) float64 {
	if math.IsNaN(rate) || math.IsInf(rate, 0) || rate <= 0 {
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

// calculateCreditedBalanceWithConversion 支持跨币种换算的入账计算（v4.6.2）。
//
// 参数：
//   - paymentAmount: 用户实际支付的金额（网关币种，gateCurrency）
//   - multiplier:    settings.balance_recharge_multiplier（仍然是 USD 计价的入账倍率）
//   - gateCurrency:  网关/支付渠道实际收款币种（CNY/USD/EUR）
//   - settlementCurrency: 用户余额的计价币种（CNY/USD/EUR）
//   - fxRate:        从 gateCurrency 换算到 settlementCurrency 的汇率（1 gateCurrency = fxRate settlementCurrency）
//
// 返回：用户到账的 settlementCurrency 余额
func calculateCreditedBalanceWithConversion(paymentAmount, multiplier float64, gateCurrency, settlementCurrency string, fxRate float64) float64 {
	m := decimal.NewFromFloat(normalizeBalanceRechargeMultiplier(multiplier))
	amt := decimal.NewFromFloat(paymentAmount).Mul(m)
	if gateCurrency != "" && settlementCurrency != "" && gateCurrency != settlementCurrency && fxRate > 0 {
		amt = amt.Mul(decimal.NewFromFloat(fxRate))
	}
	return amt.Round(payment.MaxFractionDigitsOrDefault(settlementCurrency, 2)).InexactFloat64()
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
