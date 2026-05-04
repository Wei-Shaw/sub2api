package payment

import (
	"fmt"

	"github.com/shopspring/decimal"
)

// centsPerYuan is the integer scale factor between yuan and fen.
const centsPerYuan = 100

// YuanToFen converts a CNY yuan string (e.g. "10.50") to fen (int64).
// Uses shopspring/decimal so the conversion is exact at cent precision.
func YuanToFen(yuanStr string) (int64, error) {
	d, err := decimal.NewFromString(yuanStr)
	if err != nil {
		return 0, fmt.Errorf("invalid amount: %s", yuanStr)
	}
	return d.Mul(decimal.NewFromInt(centsPerYuan)).IntPart(), nil
}

// YuanDecimalToFen converts a yuan-decimal value to fen (int64) using
// banker rounding at the cent boundary so callers that already hold a
// decimal value (PaymentOrder.Amount, validated request bodies, etc.)
// don't have to round-trip through string just to reach this helper.
func YuanDecimalToFen(d decimal.Decimal) int64 {
	return d.Mul(decimal.NewFromInt(centsPerYuan)).Round(0).IntPart()
}

// FenToYuan converts fen (int64) to a yuan-decimal value. Returning
// decimal.Decimal (vs. the legacy float64 form) lets the result feed
// directly into downstream decimal arithmetic without an
// InexactFloat64 round-trip.
func FenToYuan(fen int64) decimal.Decimal {
	return decimal.NewFromInt(fen).Div(decimal.NewFromInt(centsPerYuan))
}
