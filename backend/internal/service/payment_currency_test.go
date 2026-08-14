package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/payment"
)

func TestPaymentProviderConfigCurrencySePay(t *testing.T) {
	if got := paymentProviderConfigCurrency(payment.TypeSePay, map[string]string{}); got != "VND" {
		t.Fatalf("sepay currency = %q, want VND", got)
	}
	// SePay is VND-only: a bogus currency config must not leak CNY default.
	if got := paymentProviderConfigCurrency(payment.TypeSePay, map[string]string{"currency": "USD"}); got != "VND" {
		t.Fatalf("sepay currency with override = %q, want VND", got)
	}
}
