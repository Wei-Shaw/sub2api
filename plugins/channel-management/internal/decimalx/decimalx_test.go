// decimalx_test.go — coverage for the float64 ↔ NullDecimal helpers that
// stay local to the channel-management plugin. The proto-wire codec
// (ToProtoString / FromProtoString / FromProtoStringOrZero /
// DecimalToProtoString) used to live here too; T39 lifted it into
// `plugin-sdk/decimalx` and the round-trip / high-precision invariants
// are now exercised in plugin-sdk/decimalx/decimalx_test.go (the single
// source of truth).

package decimalx

import (
	"testing"

	"github.com/shopspring/decimal"
)

func TestFromFloat64Ptr_NilIsUnset(t *testing.T) {
	if got := FromFloat64Ptr(nil); got.Valid {
		t.Errorf("nil pointer must produce Valid=false, got %+v", got)
	}
	v := 1.5
	got := FromFloat64Ptr(&v)
	if !got.Valid {
		t.Fatalf("non-nil pointer must produce Valid=true")
	}
	if !got.Decimal.Equal(decimal.NewFromFloat(1.5)) {
		t.Errorf("decimal mismatch: %s", got.Decimal.String())
	}
}

func TestFromFloat64Ptr_ExplicitZeroIsValid(t *testing.T) {
	// Pre-T4 callers used "*float64 == 0" to mean "explicitly zero" and
	// "*float64 == nil" to mean "unset". The helper must preserve that
	// distinction or downstream price-zero overrides break.
	v := 0.0
	got := FromFloat64Ptr(&v)
	if !got.Valid {
		t.Errorf("explicit zero must be Valid=true, got %+v", got)
	}
}

func TestToFloat64Ptr_RoundTrip(t *testing.T) {
	cases := []float64{0, 1.5, 0.000003, -0.5}
	for _, v := range cases {
		nd := decimal.NullDecimal{Decimal: decimal.NewFromFloat(v), Valid: true}
		out := ToFloat64Ptr(nd)
		if out == nil {
			t.Fatalf("Valid=true must produce non-nil pointer, in=%v", v)
		}
		if *out != v {
			t.Errorf("round-trip drift: in=%v out=%v", v, *out)
		}
	}
	if got := ToFloat64Ptr(decimal.NullDecimal{}); got != nil {
		t.Errorf("Valid=false must produce nil, got %v", *got)
	}
}

func TestIsPositiveNegative(t *testing.T) {
	pos := decimal.NullDecimal{Decimal: decimal.NewFromFloat(0.5), Valid: true}
	neg := decimal.NullDecimal{Decimal: decimal.NewFromFloat(-0.5), Valid: true}
	zero := decimal.NullDecimal{Decimal: decimal.Zero, Valid: true}
	unset := decimal.NullDecimal{}

	if !IsPositive(pos) {
		t.Errorf("0.5 must be positive")
	}
	if IsPositive(neg) || IsPositive(zero) || IsPositive(unset) {
		t.Errorf("non-positive values reported as positive")
	}
	if !IsNegative(neg) {
		t.Errorf("-0.5 must be negative")
	}
	if IsNegative(pos) || IsNegative(zero) || IsNegative(unset) {
		t.Errorf("non-negative values reported as negative")
	}
}

func TestOrZero(t *testing.T) {
	v := decimal.NullDecimal{Decimal: decimal.NewFromFloat(2.5), Valid: true}
	if got := OrZero(v); !got.Equal(decimal.NewFromFloat(2.5)) {
		t.Errorf("valid pass-through: got %s", got.String())
	}
	if got := OrZero(decimal.NullDecimal{}); !got.IsZero() {
		t.Errorf("unset must produce 0, got %s", got.String())
	}
}

func TestMustPrice(t *testing.T) {
	got := MustPrice("0.001")
	if !got.Valid {
		t.Fatalf("MustPrice must produce Valid=true")
	}
	if !got.Decimal.Equal(decimal.RequireFromString("0.001")) {
		t.Errorf("decimal mismatch: %s", got.Decimal.String())
	}

	defer func() {
		if r := recover(); r == nil {
			t.Errorf("MustPrice with invalid literal must panic")
		}
	}()
	_ = MustPrice("not-a-decimal")
}
