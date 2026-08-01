//go:build unit

package provider

import (
	"math"
	"testing"
)

func TestPaddleMinorUnitsUseCurrencyPrecision(t *testing.T) {
	tests := []struct {
		name     string
		amount   string
		currency string
		want     string
	}{
		{name: "usd cents", amount: "12.34", currency: "USD", want: "1234"},
		{name: "vnd zero decimal", amount: "200000", currency: "VND", want: "200000"},
		{name: "krw zero decimal", amount: "12000", currency: "KRW", want: "12000"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := decimalAmountToMinorUnits(tt.amount, tt.currency)
			if err != nil {
				t.Fatalf("decimalAmountToMinorUnits returned error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("decimalAmountToMinorUnits(%q, %q) = %q, want %q", tt.amount, tt.currency, got, tt.want)
			}
		})
	}
}

func TestPaddleMinorUnitsToDecimalUseCurrencyPrecision(t *testing.T) {
	tests := []struct {
		name     string
		minor    string
		currency string
		want     float64
	}{
		{name: "usd cents", minor: "1234", currency: "USD", want: 12.34},
		{name: "vnd zero decimal", minor: "200000", currency: "VND", want: 200000},
		{name: "krw zero decimal", minor: "12000", currency: "KRW", want: 12000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := minorUnitsToDecimal(tt.minor, tt.currency)
			if err != nil {
				t.Fatalf("minorUnitsToDecimal returned error: %v", err)
			}
			if math.Abs(got-tt.want) > 1e-9 {
				t.Fatalf("minorUnitsToDecimal(%q, %q) = %f, want %f", tt.minor, tt.currency, got, tt.want)
			}
		})
	}
}
