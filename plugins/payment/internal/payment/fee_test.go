package payment

import (
	"testing"

	"github.com/shopspring/decimal"
)

func TestCalculatePayAmount(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		amount   string
		feeRate  string
		expected string
	}{
		{
			name:     "zero fee rate returns same amount",
			amount:   "100.00",
			feeRate:  "0",
			expected: "100.00",
		},
		{
			name:     "negative fee rate returns same amount",
			amount:   "50.00",
			feeRate:  "-5",
			expected: "50.00",
		},
		{
			name:     "1 percent fee rate",
			amount:   "100.00",
			feeRate:  "1",
			expected: "101.00",
		},
		{
			name:     "5 percent fee on 200",
			amount:   "200.00",
			feeRate:  "5",
			expected: "210.00",
		},
		{
			name:     "fee rounds UP to 2 decimal places",
			amount:   "100.00",
			feeRate:  "3",
			expected: "103.00",
		},
		{
			name:     "fee rounds UP small remainder",
			amount:   "10.00",
			feeRate:  "3.33",
			expected: "10.34",
		},
		{
			name:     "very small amount",
			amount:   "0.01",
			feeRate:  "1",
			expected: "0.02",
		},
		{
			name:     "large amount",
			amount:   "99999.99",
			feeRate:  "10",
			expected: "109999.99",
		},
		{
			name:     "100 percent fee rate doubles amount",
			amount:   "50.00",
			feeRate:  "100",
			expected: "100.00",
		},
		{
			name:     "precision 0.01 fee difference",
			amount:   "100.00",
			feeRate:  "1.01",
			expected: "101.01",
		},
		{
			name:     "precision 0.02 fee",
			amount:   "100.00",
			feeRate:  "1.02",
			expected: "101.02",
		},
		{
			name:     "zero amount with positive fee",
			amount:   "0",
			feeRate:  "5",
			expected: "0.00",
		},
		{
			name:     "fractional amount no fee",
			amount:   "19.99",
			feeRate:  "0",
			expected: "19.99",
		},
		{
			name:     "fractional fee that causes rounding up",
			amount:   "33.33",
			feeRate:  "7.77",
			expected: "35.92",
		},
		{
			// rechargeAmount 100.001 banker-rounds to 100.00 (sub-cent dust).
			// Fee must use the rounded amount as base so it does not charge
			// for the dropped 0.001 cent:
			//   amount = 100.00
			//   fee    = 100.00 * 1% = 1.0000 -> RoundUp(2) = 1.00
			//   total  = 101.00
			// Using the unrounded rechargeAmount as base would produce
			//   100.001 * 1% = 1.00001 -> RoundUp(2) = 1.01 -> 101.01
			// which charges the user a cent for sub-cent dust.
			name:     "fee base aligns with banker-rounded amount",
			amount:   "100.001",
			feeRate:  "1",
			expected: "101.00",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			amount, err := decimal.NewFromString(tt.amount)
			if err != nil {
				t.Fatalf("decimal.NewFromString(%q): %v", tt.amount, err)
			}
			rate, err := decimal.NewFromString(tt.feeRate)
			if err != nil {
				t.Fatalf("decimal.NewFromString(%q): %v", tt.feeRate, err)
			}
			got := CalculatePayAmount(amount, rate).StringFixed(2)
			if got != tt.expected {
				t.Fatalf("CalculatePayAmount(%v, %v) = %q, want %q", tt.amount, tt.feeRate, got, tt.expected)
			}
		})
	}
}
