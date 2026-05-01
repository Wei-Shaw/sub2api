// decimalx_test.go — coverage for the proto string ↔ decimal helpers
// introduced in T24 (PricingExtension wire-format migration). The goal
// is to lock in the round-trip invariant: every value the plugin emits
// via ToProtoString must come back identical through FromProtoString.
//
// Existing float64 ↔ decimal helpers (FromFloat64Ptr / ToFloat64Ptr /
// IsPositive / etc.) keep their behaviour and are exercised indirectly
// by the broader plugin / server tests, so we focus the test surface
// here on the new wire-encoding pair plus the FromProtoStringOrZero
// fast-path used on the gateway hot path.

package decimalx

import (
	"testing"

	"github.com/shopspring/decimal"
)

func TestProtoString_RoundTrip(t *testing.T) {
	cases := []string{
		"0",
		"0.000000123456789012", // 12-digit precision (matches NUMERIC(20,12))
		"1.5",
		"3.14159265358979323846",       // pi-ish; longer than IEEE-754 mantissa
		"123456789.123456789012345678", // both sides of the decimal
		"-0.000003",                    // negative small price
		"0.000003",                     // typical token cost
	}
	for _, raw := range cases {
		t.Run(raw, func(t *testing.T) {
			d := decimal.RequireFromString(raw)
			nd := decimal.NullDecimal{Decimal: d, Valid: true}

			wire := ToProtoString(nd)
			if wire == "" {
				t.Fatalf("ToProtoString returned empty for valid %q", raw)
			}

			back, err := FromProtoString(wire)
			if err != nil {
				t.Fatalf("FromProtoString(%q): unexpected error %v", wire, err)
			}
			if !back.Valid {
				t.Fatalf("FromProtoString(%q): expected Valid=true", wire)
			}
			if !back.Decimal.Equal(d) {
				t.Errorf("round-trip mismatch: in=%s wire=%s back=%s",
					raw, wire, back.Decimal.String())
			}
		})
	}
}

func TestProtoString_UnsetEncodesEmpty(t *testing.T) {
	if got := ToProtoString(decimal.NullDecimal{}); got != "" {
		t.Errorf("ToProtoString(unset) = %q, want \"\"", got)
	}
	back, err := FromProtoString("")
	if err != nil {
		t.Fatalf("FromProtoString(\"\"): unexpected error %v", err)
	}
	if back.Valid {
		t.Errorf("FromProtoString(\"\"): expected Valid=false, got %+v", back)
	}
}

func TestFromProtoString_ParseError(t *testing.T) {
	if _, err := FromProtoString("not-a-number"); err == nil {
		t.Fatal("FromProtoString(\"not-a-number\"): expected error, got nil")
	}
}

func TestFromProtoStringOrZero(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want decimal.Decimal
	}{
		{"empty", "", decimal.Zero},
		{"valid", "1.5", decimal.RequireFromString("1.5")},
		{"parse-error-falls-to-zero", "garbage", decimal.Zero},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := FromProtoStringOrZero(tc.in)
			if !got.Equal(tc.want) {
				t.Errorf("FromProtoStringOrZero(%q) = %s, want %s",
					tc.in, got.String(), tc.want.String())
			}
		})
	}
}

func TestDecimalToProtoString_ZeroEncodesAsZero(t *testing.T) {
	// DecimalToProtoString is the "no nullability" sibling of ToProtoString.
	// It must NOT collapse zero into "" because callers that hit this
	// helper want round-trip semantics (e.g. a zero result.Cost from
	// account-stats pricing is a deliberate decision, not "unset").
	if got := DecimalToProtoString(decimal.Zero); got != "0" {
		t.Errorf("DecimalToProtoString(0) = %q, want \"0\"", got)
	}
}

// TestProtoString_HighPrecision is the headline T24 invariant: a price
// with more digits than the IEEE-754 mantissa can hold flows plugin →
// wire → host without any rounding. The old `double` contract would
// round trailing digits away; the string contract keeps them verbatim.
//
// We use a 19-digit constant because that exceeds float64's ~15-17
// significant decimal digits with margin to spare; values that fit
// inside the mantissa happen to survive a float round-trip and would
// not exercise the difference between the two contracts. The value is
// also chosen with no trailing zero so decimal.Decimal.String()'s
// canonicalisation does not strip any digits.
func TestProtoString_HighPrecision(t *testing.T) {
	const raw = "0.1234567890123456789" // 19 significant digits, no trailing zero
	d := decimal.RequireFromString(raw)
	nd := decimal.NullDecimal{Decimal: d, Valid: true}

	wire := ToProtoString(nd)
	if wire != raw {
		t.Errorf("ToProtoString: canonical form changed: in=%s wire=%s", raw, wire)
	}

	back, err := FromProtoString(wire)
	if err != nil {
		t.Fatalf("FromProtoString(%q): %v", wire, err)
	}
	if !back.Decimal.Equal(d) {
		t.Errorf("round-trip changed value: in=%s back=%s",
			d.String(), back.Decimal.String())
	}

	// Sanity check: the same value through the legacy float64 channel
	// rounds away the precision — proves the test is meaningful and
	// that the new wire contract is strictly more precise than the old
	// `double` one. If float happens to round-trip this exact value the
	// test loses its safety net, so flag it explicitly.
	floatRoundTrip := decimal.NewFromFloat(d.InexactFloat64())
	if floatRoundTrip.Equal(d) {
		t.Errorf("float64 path unexpectedly preserved precision (%s); "+
			"the test is no longer detecting a difference between "+
			"string and double encodings",
			floatRoundTrip.String())
	}
}
