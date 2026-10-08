//go:build unit

package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/payment"
)

func TestBuildProviderCreatePaymentRequestCarriesSelectedCurrency(t *testing.T) {
	sel := &payment.InstanceSelection{
		ProviderKey: payment.TypeSepay,
		Config:      map[string]string{"currency": "vnd"},
	}

	got := buildProviderCreatePaymentRequest(CreateOrderRequest{PaymentType: payment.TypeSepay}, sel, "sub2_20250409aB3kX9mQ", "200000", "Sub2API 200000 VND")
	if got.PaymentCurrency != "VND" {
		t.Fatalf("PaymentCurrency = %q, want VND", got.PaymentCurrency)
	}
}
