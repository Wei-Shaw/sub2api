package decimalx

import (
	"strings"
	"testing"

	"github.com/shopspring/decimal"
)

// TestRoundTrip_NullDecimal — every wire string produced by ToProtoString must
// round-trip back through FromProtoString to the same NullDecimal.
func TestRoundTrip_NullDecimal(t *testing.T) {
	cases := []string{"0.001", "1", "1.5", "0.000000000001", "1234567890.123456"}
	for _, in := range cases {
		nd, err := FromProtoString(in)
		if err != nil {
			t.Fatalf("FromProtoString(%q): unexpected error %v", in, err)
		}
		if !nd.Valid {
			t.Fatalf("FromProtoString(%q): expected Valid=true", in)
		}
		wire := ToProtoString(nd)
		back, err := FromProtoString(wire)
		if err != nil {
			t.Fatalf("FromProtoString(%q): unexpected error %v", wire, err)
		}
		if !back.Decimal.Equal(nd.Decimal) {
			t.Errorf("round trip drift: %q -> %q -> %s", in, wire, back.Decimal.String())
		}
	}
}

// TestEmptyMeansUnset — empty string is the canonical "unset" wire value;
// it must produce an invalid NullDecimal both ways.
func TestEmptyMeansUnset(t *testing.T) {
	got := ToProtoString(decimal.NullDecimal{})
	if got != "" {
		t.Errorf("ToProtoString(invalid)=%q, want \"\"", got)
	}
	nd, err := FromProtoString("")
	if err != nil {
		t.Fatalf("FromProtoString(\"\"): unexpected error %v", err)
	}
	if nd.Valid {
		t.Errorf("FromProtoString(\"\"): want Valid=false, got %+v", nd)
	}
}

// TestFromProtoString_ParseError — malformed input must surface an error
// wrapping the offending raw text so operators can grep it from logs.
func TestFromProtoString_ParseError(t *testing.T) {
	_, err := FromProtoString("not-a-number")
	if err == nil {
		t.Fatal("FromProtoString(\"not-a-number\"): expected error")
	}
	if !strings.Contains(err.Error(), "not-a-number") {
		t.Errorf("error %q must contain the raw input", err.Error())
	}
}

// TestFromProtoStringOrZero_DegradesOnError — best-effort decode never panics
// or returns an error; malformed input maps to decimal.Zero.
func TestFromProtoStringOrZero_DegradesOnError(t *testing.T) {
	if got := FromProtoStringOrZero("not-a-number"); !got.IsZero() {
		t.Errorf("FromProtoStringOrZero(bad)=%s, want 0", got.String())
	}
	if got := FromProtoStringOrZero(""); !got.IsZero() {
		t.Errorf("FromProtoStringOrZero(\"\")=%s, want 0", got.String())
	}
	if got := FromProtoStringOrZero("1.25"); !got.Equal(decimal.NewFromFloat(1.25)) {
		t.Errorf("FromProtoStringOrZero(1.25)=%s, want 1.25", got.String())
	}
}

// TestFormatFloatString_ZeroMeansUnset — the FormatFloatString contract is
// "0 -> empty string", matching the NullDecimal Valid=false convention.
func TestFormatFloatString_ZeroMeansUnset(t *testing.T) {
	if got := FormatFloatString(0); got != "" {
		t.Errorf("FormatFloatString(0)=%q, want \"\"", got)
	}
	if got := FormatFloatString(0.001); got == "" {
		t.Errorf("FormatFloatString(0.001) must not be empty")
	}
}

// TestParseFloatString_DegradesOnError — host hot path requirement; parse
// errors degrade to 0 instead of propagating.
func TestParseFloatString_DegradesOnError(t *testing.T) {
	if got := ParseFloatString("bogus"); got != 0 {
		t.Errorf("ParseFloatString(bogus)=%v, want 0", got)
	}
	if got := ParseFloatString(""); got != 0 {
		t.Errorf("ParseFloatString(\"\")=%v, want 0", got)
	}
	if got := ParseFloatString("1.5"); got != 1.5 {
		t.Errorf("ParseFloatString(1.5)=%v, want 1.5", got)
	}
}

// TestFloatRoundTrip_TolerantOfIEEE754 — verifies the wire round-trips a
// host-side float64 input without nasty trailing-nines drift.
func TestFloatRoundTrip_TolerantOfIEEE754(t *testing.T) {
	cases := []float64{0.001, 1.5, 1234567.89, 0.000000001}
	for _, f := range cases {
		wire := FormatFloatString(f)
		got := ParseFloatString(wire)
		// Allow a single ULP of drift; the contract is "no nasty trailing 9s",
		// not bit-exact float64.
		diff := got - f
		if diff < 0 {
			diff = -diff
		}
		if diff > 1e-9 {
			t.Errorf("float round-trip drift: %v -> %q -> %v", f, wire, got)
		}
	}
}

// TestDecimalToProtoString_ZeroIsLiteralZero — D variant emits "0", NOT "",
// to disambiguate "explicit zero cost" from "cost not set".
func TestDecimalToProtoString_ZeroIsLiteralZero(t *testing.T) {
	if got := DecimalToProtoString(decimal.Zero); got != "0" {
		t.Errorf("DecimalToProtoString(0)=%q, want \"0\"", got)
	}
}
