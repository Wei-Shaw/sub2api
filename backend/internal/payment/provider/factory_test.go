package provider

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/payment"
)

func TestCreateProviderSePay(t *testing.T) {
	p, err := CreateProvider(payment.TypeSePay, "7", sepayTestConfig())
	if err != nil {
		t.Fatal(err)
	}
	if p.ProviderKey() != payment.TypeSePay {
		t.Fatalf("provider key = %q", p.ProviderKey())
	}
	if _, err := CreateProvider(payment.TypeSePay, "7", map[string]string{}); err == nil {
		t.Fatal("expected config validation error from factory")
	}
}
