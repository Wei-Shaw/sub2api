package payment

import (
	"fmt"

	"github.com/shopspring/decimal"
)

const centsPerYuan = 100

// YuanToFen converts a CNY yuan string (e.g. "10.50") to fen (int64).
// Uses shopspring/decimal for precision.
func YuanToFen(yuanStr string) (int64, error) {
	d, err := decimal.NewFromString(yuanStr)
	if err != nil {
		return 0, fmt.Errorf("invalid amount: %s", yuanStr)
	}
	return d.Mul(decimal.NewFromInt(centsPerYuan)).IntPart(), nil
}

// FenToYuan converts fen (int64) to yuan as a float64 for interface compatibility.
func FenToYuan(fen int64) float64 {
	return decimal.NewFromInt(fen).Div(decimal.NewFromInt(centsPerYuan)).InexactFloat64()
}
