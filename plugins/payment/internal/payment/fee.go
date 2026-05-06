package payment

import (
	"github.com/shopspring/decimal"
)

// CalculatePayAmount computes the total pay amount given a recharge amount and
// fee rate (percentage). Fee = amount * feeRate / 100, rounded UP (away from zero)
// to 2 decimal places. The returned decimal carries exact cent precision so
// callers may format it (StringFixed(2)) or feed it back into further decimal
// arithmetic without a float64 round-trip.
//
// If feeRate is non-positive the amount is returned unchanged at cent precision.
func CalculatePayAmount(rechargeAmount, feeRate decimal.Decimal) decimal.Decimal {
	amount := rechargeAmount.RoundBank(2)
	if feeRate.Sign() <= 0 {
		return amount
	}
	fee := amount.Mul(feeRate).Div(decimal.NewFromInt(100)).RoundUp(2)
	return amount.Add(fee)
}
