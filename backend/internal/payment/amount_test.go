//go:build unit

package payment

import (
	"math"
	"testing"
)

func TestPaymentCurrencyHelpers(t *testing.T) {
	tests := []struct {
		name      string
		currency  string
		amount    string
		wantMinor int64
		wantBack  float64
	}{
		{name: "hkd uses cents", currency: "hkd", amount: "12.34", wantMinor: 1234, wantBack: 12.34},
		{name: "jpy has no minor unit", currency: "JPY", amount: "12", wantMinor: 12, wantBack: 12},
		{name: "kwd uses three decimal minor units", currency: "KWD", amount: "12.345", wantMinor: 12345, wantBack: 12.345},
		{name: "isk uses the legacy two-decimal API amount", currency: "ISK", amount: "12", wantMinor: 1200, wantBack: 12},
		{name: "ugx uses the legacy two-decimal API amount", currency: "UGX", amount: "12.00", wantMinor: 1200, wantBack: 12},
		{name: "empty currency defaults to the gateway currency", currency: "", amount: "1230", wantMinor: 1230, wantBack: 1230},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := AmountToMinorUnit(tt.amount, tt.currency)
			if err != nil {
				t.Fatalf("AmountToMinorUnit(%q, %q) unexpected error: %v", tt.amount, tt.currency, err)
			}
			if got != tt.wantMinor {
				t.Fatalf("AmountToMinorUnit(%q, %q) = %d, want %d", tt.amount, tt.currency, got, tt.wantMinor)
			}
			back := MinorUnitToAmount(got, tt.currency)
			if math.Abs(back-tt.wantBack) > 1e-9 {
				t.Fatalf("MinorUnitToAmount(%d, %q) = %f, want %f", got, tt.currency, back, tt.wantBack)
			}
		})
	}
}

func TestFormatAmountForCurrency(t *testing.T) {
	tests := []struct {
		currency string
		amount   float64
		want     string
	}{
		{currency: "CNY", amount: 12.3, want: "12.30"},
		{currency: "JPY", amount: 12, want: "12"},
		{currency: "KWD", amount: 12.345, want: "12.345"},
		{currency: "ISK", amount: 12, want: "12"},
	}

	for _, tt := range tests {
		t.Run(tt.currency, func(t *testing.T) {
			if got := FormatAmountForCurrency(tt.amount, tt.currency); got != tt.want {
				t.Fatalf("FormatAmountForCurrency(%v, %q) = %q, want %q", tt.amount, tt.currency, got, tt.want)
			}
		})
	}
}

func TestAmountToMinorUnitRejectsUnsupportedPrecision(t *testing.T) {
	if _, err := AmountToMinorUnit("100.50", "JPY"); err == nil {
		t.Fatal("expected fractional JPY amount to fail")
	}
	if _, err := AmountToMinorUnit("100.50", "ISK"); err == nil {
		t.Fatal("expected fractional ISK amount to fail")
	}
	if _, err := AmountToMinorUnit("100.50", "UGX"); err == nil {
		t.Fatal("expected fractional UGX amount to fail")
	}
	if _, err := AmountToMinorUnit("12.345", "HKD"); err == nil {
		t.Fatal("expected amount with more than two decimal places to fail")
	}
	if _, err := AmountToMinorUnit("12.3456", "KWD"); err == nil {
		t.Fatal("expected amount with more than three decimal places to fail")
	}
	if got, err := AmountToMinorUnit("100.00", "JPY"); err != nil || got != 100 {
		t.Fatalf("AmountToMinorUnit integer-form JPY = (%d, %v), want (100, nil)", got, err)
	}
}

func TestThreeDecimalPaymentCurrencies(t *testing.T) {
	for _, currency := range []string{"BHD", "IQD", "JOD", "KWD", "LYD", "OMR", "TND"} {
		t.Run(currency, func(t *testing.T) {
			got, err := AmountToMinorUnit("12.345", currency)
			if err != nil {
				t.Fatalf("AmountToMinorUnit(%q, %q) unexpected error: %v", "12.345", currency, err)
			}
			if got != 12345 {
				t.Fatalf("AmountToMinorUnit(%q, %q) = %d, want 12345", "12.345", currency, got)
			}
			if back := MinorUnitToAmount(got, currency); math.Abs(back-12.345) > 1e-9 {
				t.Fatalf("MinorUnitToAmount(%d, %q) = %f, want 12.345", got, currency, back)
			}
		})
	}
}

func TestNormalizePaymentCurrencyRejectsInvalidCodes(t *testing.T) {
	if _, err := NormalizePaymentCurrency("HK"); err == nil {
		t.Fatal("expected invalid two-letter currency to fail")
	}
	if _, err := NormalizePaymentCurrency("US1"); err == nil {
		t.Fatal("expected non-letter currency to fail")
	}
}
