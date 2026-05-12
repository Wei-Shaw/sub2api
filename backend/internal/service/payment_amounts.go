package service

import (
	"math"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/Wei-Shaw/sub2api/internal/pkg/moneyx"
	"github.com/shopspring/decimal"
)

const defaultBalanceRechargeMultiplier = 1.0

func normalizeBalanceRechargeMultiplier(multiplier float64) float64 {
	if math.IsNaN(multiplier) || math.IsInf(multiplier, 0) || multiplier <= 0 {
		return defaultBalanceRechargeMultiplier
	}
	return multiplier
}

func calculateCreditedBalance(paymentAmount, multiplier float64) float64 {
	return moneyx.Currency(paymentAmount).
		Mul(decimal.NewFromFloat(normalizeBalanceRechargeMultiplier(multiplier))).
		Round(2).
		InexactFloat64()
}

func refundableOrderPayAmount(orderAmount, payAmount float64) float64 {
	if payAmount > 0 {
		return moneyx.Currency(payAmount).InexactFloat64()
	}
	if orderAmount > 0 {
		return moneyx.Currency(orderAmount).InexactFloat64()
	}
	return 0
}

func calculateBalanceDeductionAmount(orderAmount, payAmount, refundAmount float64) float64 {
	refundablePayAmount := refundableOrderPayAmount(orderAmount, payAmount)
	if orderAmount <= 0 || refundablePayAmount <= 0 || refundAmount <= 0 {
		return 0
	}
	orderAmountDec := moneyx.Currency(orderAmount)
	refundAmountDec := moneyx.Currency(refundAmount)
	refundablePayAmountDec := moneyx.Currency(refundablePayAmount)
	if refundAmountDec.GreaterThanOrEqual(refundablePayAmountDec) {
		return orderAmountDec.InexactFloat64()
	}
	return orderAmountDec.
		Mul(refundAmountDec).
		Div(refundablePayAmountDec).
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
