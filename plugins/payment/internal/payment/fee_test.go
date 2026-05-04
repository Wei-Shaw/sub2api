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
