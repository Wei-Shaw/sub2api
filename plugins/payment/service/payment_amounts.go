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

// yuanToFen converts a CNY yuan float64 to fen (int64) using
// shopspring/decimal so the conversion is exact at the cent boundary.
// Negative / NaN / Inf inputs return 0 — callers that have not already
// validated the input should pre-check via isValidProviderAmount.
//
// This is the canonical helper across the service layer; webhook amount
// comparison, order event publication, balance fulfillment events and
// admin refund validation all share it so the rounding rule cannot drift.
func yuanToFen(yuan float64) int64 {
	return decimal.NewFromFloat(yuan).Mul(decimal.NewFromInt(100)).Round(0).IntPart()
}

// yuanToFenInt is an alias for yuanToFen used at event-publication sites
// where the call reads more naturally as a one-shot conversion than a
// reusable helper. Both forms compile to the same code.
func yuanToFenInt(yuan float64) int64 { return yuanToFen(yuan) }

// roundYuan rounds a CNY value to 2 decimals using shopspring/decimal.
// The float64 round-trip via InexactFloat64 still defeats exact decimal
// representation but the *rounding* itself is correct, which is what
// matters for display / aggregate stats (the alternative
// math.Round(v*100)/100 is wrong for values like 19.985).
func roundYuan(v float64) float64 {
	return decimal.NewFromFloat(v).Round(2).InexactFloat64()
}

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
	// Use decimal-fen comparison to avoid float drift: when the refund
	// equals the order amount at cent precision the gateway refund is
	// exactly payAmount (no ratio multiplication).
	if yuanToFen(refundAmount) == yuanToFen(orderAmount) {
		return decimal.NewFromFloat(payAmount).Round(2).InexactFloat64()
	}
	return decimal.NewFromFloat(payAmount).
		Mul(decimal.NewFromFloat(refundAmount)).
		Div(decimal.NewFromFloat(orderAmount)).
		Round(2).
		InexactFloat64()
}
