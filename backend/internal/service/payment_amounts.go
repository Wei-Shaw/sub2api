package service

import (
	"math"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
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

// normalizeSubscriptionUSDToVNDRate 将非法值归一为 0（换算关闭）。
func normalizeSubscriptionUSDToVNDRate(rate float64) float64 {
	return normalizeSubscriptionUSDToCNYRate(rate)
}

// calculateRechargeCreditedBalance converts a recharge amount in the method
// currency into the panel's USD-denominated balance. VND methods divide by
// the configured USD→VND rate (the recharge multiplier still applies on top);
// other currencies keep the legacy multiplier-only behavior.
func calculateRechargeCreditedBalance(payAmount float64, methodCurrency string, cfg *PaymentConfig) (float64, error) {
	if methodCurrency == payment.CurrencyVND {
		rate := normalizeSubscriptionUSDToVNDRate(cfg.SubscriptionUSDToVNDRate)
		if rate <= 0 {
			return 0, infraerrors.BadRequest("RECHARGE_VND_RATE_REQUIRED",
				"balance recharge via VND methods requires the USD to VND rate to be configured")
		}
		payAmount = decimal.NewFromFloat(payAmount).Div(decimal.NewFromFloat(rate)).InexactFloat64()
	}
	return calculateCreditedBalance(payAmount, cfg.BalanceRechargeMultiplier), nil
}

func calculateCreditedBalance(paymentAmount, multiplier float64) float64 {
	return decimal.NewFromFloat(paymentAmount).
		Mul(decimal.NewFromFloat(normalizeBalanceRechargeMultiplier(multiplier))).
		Round(2).
		InexactFloat64()
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
