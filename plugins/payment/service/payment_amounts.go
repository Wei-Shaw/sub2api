// Package service is the payment plugin's service layer. Files here
// are progressively ported from backend/internal/service/payment_*.go
// to the plugin SDK. Files still under the payment_services_wip build
// tag have not been adapted yet — see plugins/payment/README or the
// plugin migration tracking issue for status.
package service

import (
	"math"

	"github.com/shopspring/decimal"
)

const defaultBalanceRechargeMultiplier = 1.0

// amountToleranceCNY is the rounding tolerance used by refund amount
// computation. Values within this tolerance of the order amount are
// treated as a full refund and the gateway refund equals payAmount
// directly (avoids a 0.01 CNY drift introduced by ratio multiplication).
const amountToleranceCNY = 0.01

func normalizeBalanceRechargeMultiplier(multiplier float64) float64 {
	if math.IsNaN(multiplier) || math.IsInf(multiplier, 0) || multiplier <= 0 {
		return defaultBalanceRechargeMultiplier
	}
	return multiplier
}

func calculateCreditedBalance(paymentAmount, multiplier float64) float64 {
	return decimal.NewFromFloat(paymentAmount).
		Mul(decimal.NewFromFloat(normalizeBalanceRechargeMultiplier(multiplier))).
		Round(2).
		InexactFloat64()
}

func calculateGatewayRefundAmount(orderAmount, payAmount, refundAmount float64) float64 {
	if orderAmount <= 0 || payAmount <= 0 || refundAmount <= 0 {
		return 0
	}
	if math.Abs(refundAmount-orderAmount) <= amountToleranceCNY {
		return decimal.NewFromFloat(payAmount).Round(2).InexactFloat64()
	}
	return decimal.NewFromFloat(payAmount).
		Mul(decimal.NewFromFloat(refundAmount)).
		Div(decimal.NewFromFloat(orderAmount)).
		Round(2).
		InexactFloat64()
}
