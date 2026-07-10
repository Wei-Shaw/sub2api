package service

import "testing"

func TestConvertBillingAmount_USDCNY(t *testing.T) {
	const rate = DefaultUSDCNYRate

	tests := []struct {
		name     string
		amount   float64
		from     string
		to       string
		expected float64
	}{
		{name: "cny to usd", amount: 7.2, from: BillingCurrencyCNY, to: BillingCurrencyUSD, expected: 1.0},
		{name: "usd to cny", amount: 1.25, from: BillingCurrencyUSD, to: BillingCurrencyCNY, expected: 9.0},
		{name: "same currency", amount: 4.2, from: BillingCurrencyCNY, to: BillingCurrencyCNY, expected: 4.2},
		{name: "blank currency defaults usd", amount: 3.5, from: "", to: BillingCurrencyUSD, expected: 3.5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ConvertBillingAmount(tt.amount, tt.from, tt.to, rate)
			if !approxEqual(got, tt.expected) {
				t.Fatalf("ConvertBillingAmount() = %.8f, want %.8f", got, tt.expected)
			}
		})
	}
}

func TestConvertBillingAmount_InvalidRateLeavesCrossCurrencyAmountUnchanged(t *testing.T) {
	got := ConvertBillingAmount(7.2, BillingCurrencyCNY, BillingCurrencyUSD, 0)
	if got != 7.2 {
		t.Fatalf("ConvertBillingAmount invalid rate = %.8f, want original amount", got)
	}
}
