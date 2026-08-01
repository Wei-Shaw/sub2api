//go:build unit

package service

import (
	"math"
	"testing"
)

func TestCurrencyMinorUnits(t *testing.T) {
	tests := []struct {
		currency string
		want     int
	}{
		{currency: "VND", want: 0},
		{currency: "KRW", want: 0},
		{currency: "JPY", want: 0},
		{currency: "USD", want: 2},
		{currency: "CNY", want: 2},
		{currency: "EUR", want: 2},
		{currency: "  usd  ", want: 2},
	}

	for _, tc := range tests {
		if got := currencyMinorUnits(tc.currency); got != tc.want {
			t.Fatalf("currencyMinorUnits(%q) = %d, want %d", tc.currency, got, tc.want)
		}
	}
}

func TestCurrencyRoundingRules(t *testing.T) {
	tests := []struct {
		name     string
		got      float64
		expected float64
	}{
		{name: "VND collection rounds up to whole unit", got: roundPaymentAmountForCollection(1234.01, "VND"), expected: 1235},
		{name: "VND collection ignores float noise only", got: roundPaymentAmountForCollection(1234.000000001, "VND"), expected: 1234},
		{name: "VND collection does not under collect meaningful fraction", got: roundPaymentAmountForCollection(1234.004, "VND"), expected: 1235},
		{name: "USD collection rounds up to cents", got: roundPaymentAmountForCollection(12.341, "USD"), expected: 12.35},
		{name: "CNY collection rounds up to cents", got: roundPaymentAmountForCollection(10.001, "CNY"), expected: 10.01},
		{name: "USD ledger credit rounds down to cents", got: roundLedgerAmountForCredit(9.999, "USD"), expected: 9.99},
		{name: "USD ledger credit ignores float noise only", got: roundLedgerAmountForCredit(9.99999999, "USD"), expected: 10.00},
		{name: "KRW ledger credit rounds down to whole unit", got: roundLedgerAmountForCredit(1000.99, "KRW"), expected: 1000},
	}

	for _, tc := range tests {
		if !floatAmountEqual(tc.got, tc.expected) {
			t.Fatalf("%s = %v, want %v", tc.name, tc.got, tc.expected)
		}
	}
}

func floatAmountEqual(got, expected float64) bool {
	return math.Abs(got-expected) < 1e-9
}

func TestFXConvertPaymentToLedger(t *testing.T) {
	snapshot := fxSnapshot{
		PaymentCurrency:     "VND",
		LedgerCurrency:      "USD",
		RatePaymentToLedger: 0.000039215686,
	}

	got, err := convertPaymentToLedger(255000, snapshot)
	if err != nil {
		t.Fatalf("convertPaymentToLedger() error = %v", err)
	}
	if !floatAmountEqual(got, 10.00) {
		t.Fatalf("convertPaymentToLedger() = %v, want 10.00", got)
	}
}

func TestFXConvertLedgerToPayment(t *testing.T) {
	snapshot := fxSnapshot{
		PaymentCurrency:     "VND",
		LedgerCurrency:      "USD",
		RatePaymentToLedger: 10.0 / 255000.0,
	}

	got, err := convertLedgerToPayment(10, snapshot)
	if err != nil {
		t.Fatalf("convertLedgerToPayment() error = %v", err)
	}
	if !floatAmountEqual(got, 255000) {
		t.Fatalf("convertLedgerToPayment() = %v, want 255000", got)
	}
}

func TestFXConvertLedgerToPaymentDoesNotUnderCollectWithTruncatedRate(t *testing.T) {
	snapshot := fxSnapshot{
		PaymentCurrency:     "VND",
		LedgerCurrency:      "USD",
		RatePaymentToLedger: 0.000039215686,
	}

	got, err := convertLedgerToPayment(10, snapshot)
	if err != nil {
		t.Fatalf("convertLedgerToPayment() error = %v", err)
	}
	if !floatAmountEqual(got, 255001) {
		t.Fatalf("convertLedgerToPayment() = %v, want 255001 to avoid under-collection", got)
	}
}

func TestFXConvertRejectsInvalidRates(t *testing.T) {
	tests := []float64{0, -1, math.NaN()}
	for _, rate := range tests {
		snapshot := fxSnapshot{PaymentCurrency: "VND", LedgerCurrency: "USD", RatePaymentToLedger: rate}

		if _, err := convertPaymentToLedger(1000, snapshot); err == nil {
			t.Fatalf("convertPaymentToLedger() with rate=%v error = nil, want error", rate)
		}
		if _, err := convertLedgerToPayment(10, snapshot); err == nil {
			t.Fatalf("convertLedgerToPayment() with rate=%v error = nil, want error", rate)
		}
	}
}

func TestFXConvertRejectsInvalidAmounts(t *testing.T) {
	snapshot := fxSnapshot{PaymentCurrency: "VND", LedgerCurrency: "USD", RatePaymentToLedger: 10.0 / 255000.0}
	invalidAmounts := []float64{math.NaN(), math.Inf(1), math.Inf(-1)}
	for _, amount := range invalidAmounts {
		if _, err := convertPaymentToLedger(amount, snapshot); err == nil {
			t.Fatalf("convertPaymentToLedger() with amount=%v error = nil, want error", amount)
		}
		if _, err := convertLedgerToPayment(amount, snapshot); err == nil {
			t.Fatalf("convertLedgerToPayment() with amount=%v error = nil, want error", amount)
		}
	}
}
