// Package service is the payment plugin's service layer. Files here
// are progressively ported from backend/internal/service/payment_*.go
// to the plugin SDK. Files still under the payment_services_wip build
// tag have not been adapted yet — see plugins/payment/README or the
// plugin migration tracking issue for status.
package service

import (
	"github.com/shopspring/decimal"

	"github.com/Wei-Shaw/sub2api/plugins/payment/internal/payment"
)

// defaultBalanceRechargeMultiplier is the multiplier applied to balance
// recharges when the operator has not configured one. Kept as a
// decimal so all downstream multiplication paths share one type.
var defaultBalanceRechargeMultiplier = decimal.NewFromInt(1)

// yuanToFen converts a yuan-decimal value to integer fen using the
// canonical helper exposed by the internal/payment package.
//
// Centralising the conversion here keeps the rounding rule visible
// across webhook amount comparison, order event publication, balance
// fulfillment events and admin refund validation; all of those used to
// share a yuanToFen / yuanToFenInt pair on top of float64. The decimal
// version no longer needs the float wrapper.
func yuanToFen(d decimal.Decimal) int64 {
	return payment.YuanDecimalToFen(d)
}

// roundYuan rounds a CNY decimal to two decimal places.
//
// Used by the dashboard / stats helpers when accumulating sums — the
// underlying field is already decimal(20,2) so the rounding is normally
// a no-op, but keeping it explicit makes the assembly site obviously
// safe (the alternative math.Round(v*100)/100 was wrong for values like
// 19.985).
func roundYuan(d decimal.Decimal) decimal.Decimal {
	return d.Round(2)
}

// normalizeBalanceRechargeMultiplier rejects non-positive (or
// uninitialised) multipliers and falls back to 1. Decimal does not
// support NaN/Inf so the previous math.IsNaN / math.IsInf checks are no
// longer required.
func normalizeBalanceRechargeMultiplier(m decimal.Decimal) decimal.Decimal {
	if !m.IsPositive() {
		return defaultBalanceRechargeMultiplier
	}
	return m
}

// calculateCreditedBalance applies the recharge bonus multiplier to the
// raw payment amount. Result is rounded to two decimals for storage.
func calculateCreditedBalance(paymentAmount, multiplier decimal.Decimal) decimal.Decimal {
	return paymentAmount.Mul(normalizeBalanceRechargeMultiplier(multiplier)).Round(2)
}

// calculateGatewayRefundAmount returns the amount the upstream gateway
// should refund given an (orderAmount, payAmount, refundAmount) triple
// expressed in CNY. The intuition: the gateway saw payAmount; we owe
// the user back the gateway-side proportion of the in-system refund
// amount, with cent precision.
//
// When refund == orderAmount at cent precision we short-circuit to
// payAmount unchanged so a full refund never drops a cent due to
// reciprocal rounding.
func calculateGatewayRefundAmount(orderAmount, payAmount, refundAmount decimal.Decimal) decimal.Decimal {
	if orderAmount.Sign() <= 0 || payAmount.Sign() <= 0 || refundAmount.Sign() <= 0 {
		return decimal.Zero
	}
	if yuanToFen(refundAmount) == yuanToFen(orderAmount) {
		return payAmount.Round(2)
	}
	return payAmount.Mul(refundAmount).Div(orderAmount).Round(2)
}
