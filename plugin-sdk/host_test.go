package pluginsdk

import (
	"context"
	"errors"
	"testing"
)

func TestParseOptionalDecimal_Empty(t *testing.T) {
	got := parseOptionalDecimal("")
	if got != nil {
		t.Errorf("parseOptionalDecimal(\"\") = %v, want nil", got)
	}
}

func TestParseOptionalDecimal_Valid(t *testing.T) {
	got := parseOptionalDecimal("1.5")
	if got == nil {
		t.Fatal("parseOptionalDecimal(\"1.5\") = nil, want non-nil")
	}
	if got.String() != "1.5" {
		t.Errorf("parseOptionalDecimal(\"1.5\") = %s, want 1.5", got.String())
	}
}

func TestParseOptionalDecimal_Invalid(t *testing.T) {
	got := parseOptionalDecimal("invalid")
	if got != nil {
		t.Errorf("parseOptionalDecimal(\"invalid\") = %v, want nil (graceful fallback)", got)
	}
}

func TestNilHostClient_ResolveModelPricing(t *testing.T) {
	var c nilHostClient
	pricing, err := c.ResolveModelPricing(context.Background(), "any-model")
	if pricing != nil {
		t.Errorf("nilHostClient.ResolveModelPricing pricing = %v, want nil", pricing)
	}
	if !errors.Is(err, ErrHostPricingUnavailable) {
		t.Errorf("nilHostClient.ResolveModelPricing err = %v, want %v", err, ErrHostPricingUnavailable)
	}
}
