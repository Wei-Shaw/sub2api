package payment

import (
	"testing"
)

func TestCalculatePayAmount(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		amount   float64
		feeRate  float64
		expected string
	}{
		{
			name:     "zero fee rate returns same amount",
			amount:   100.00,
			feeRate:  0,
			expected: "100.00",
		},
		{
			name:     "negative fee rate returns same amount",
			amount:   50.00,
			feeRate:  -5,
			expected: "50.00",
		},
		{
			name:     "1 percent fee rate",
			amount:   100.00,
			feeRate:  1,
			expected: "101.00",
		},
		{
			name:     "5 percent fee on 200",
			amount:   200.00,
			feeRate:  5,
			expected: "210.00",
		},
		{
			name:     "fee rounds UP to 2 decimal places",
			amount:   100.00,
			feeRate:  3,
			expected: "103.00",
		},
		{
			name:     "fee rounds UP small remainder",
			amount:   10.00,
			feeRate:  3.33,
			expected: "10.34", // 10 * 3.33 / 100 = 0.333 -> round up -> 0.34
		},
		{
			name:     "very small amount",
			amount:   0.01,
			feeRate:  1,
			expected: "0.02", // 0.01 * 1/100 = 0.0001 -> round up -> 0.01 -> total 0.02
		},
		{
			name:     "large amount",
			amount:   99999.99,
			feeRate:  10,
			expected: "109999.99", // 99999.99 * 10/100 = 9999.999 -> round up -> 10000.00 -> total 109999.99
		},
		{
			name:     "100 percent fee rate doubles amount",
			amount:   50.00,
			feeRate:  100,
			expected: "100.00",
		},
		{
			name:     "precision 0.01 fee difference",
			amount:   100.00,
			feeRate:  1.01,
			expected: "101.01", // 100 * 1.01/100 = 1.01
		},
		{
			name:     "precision 0.02 fee",
			amount:   100.00,
			feeRate:  1.02,
			expected: "101.02",
		},
		{
			name:     "zero amount with positive fee",
			amount:   0,
			feeRate:  5,
			expected: "0.00",
		},
		{
			name:     "fractional amount no fee",
			amount:   19.99,
			feeRate:  0,
			expected: "19.99",
		},
		{
			name:     "fractional fee that causes rounding up",
			amount:   33.33,
			feeRate:  7.77,
			expected: "35.92", // 33.33 * 7.77 / 100 = 2.589741 -> round up -> 2.59 -> total 35.92
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := CalculatePayAmount(tt.amount, tt.feeRate)
			if got != tt.expected {
				t.Fatalf("CalculatePayAmount(%v, %v) = %q, want %q", tt.amount, tt.feeRate, got, tt.expected)
			}
		})
	}
}

func TestCalculatePayAmountForCurrency(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		amount   float64
		feeRate  float64
		currency string
		expected string
	}{
		{
			name:     "zero decimal currency rounds fee up to whole unit",
			amount:   100,
			feeRate:  2.5,
			currency: "JPY",
			expected: "103",
		},
		{
			name:     "three decimal currency keeps three decimal places",
			amount:   12.345,
			feeRate:  1,
			currency: "KWD",
			expected: "12.469",
		},
		{
			name:     "stripe legacy zero decimal currency displays whole unit",
			amount:   100,
			feeRate:  2.5,
			currency: "ISK",
			expected: "103",
		},
		{
			name:     "default currency keeps existing two decimal behavior",
			amount:   10,
			feeRate:  3.33,
			currency: "CNY",
			expected: "10.34",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := CalculatePayAmountForCurrency(tt.amount, tt.feeRate, tt.currency)
			if got != tt.expected {
				t.Fatalf("CalculatePayAmountForCurrency(%v, %v, %q) = %q, want %q", tt.amount, tt.feeRate, tt.currency, got, tt.expected)
			}
		})
	}
}
