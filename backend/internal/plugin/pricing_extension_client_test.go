// pricing_extension_client_test.go — coverage for the host-side
// PricingExtensionClient wire decoders. T24 migrated PricingExtension
// price fields from `double` to string decimals, so the headline test
// here is the round-trip invariant: a decimal string emitted by the
// plugin (via decimalx.ToProtoString) decodes through protoToOverride
// without IEEE-754 rounding sneaking in at this boundary.
//
// We also verify the helpers used on the AdjustCost / ResolveAccountStats
// paths so a future regression to e.g. `final.GetTotal()` (the old
// float64 accessor) fails to compile or test instead of silently
// reverting the contract.

package plugin

import (
	"log/slog"
	"math"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/Wei-Shaw/sub2api/plugin-sdk/decimalx"
	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
)

// approxEqual lets us assert decimal-string round-trips ended up at a
// float64 value matching the exact mantissa decimal would have rounded to.
// We allow a 1-ULP tolerance because parseDecimalString → InexactFloat64
// is the documented bounded loss for the host's float64 cache layer.
func approxEqual(a, b float64) bool {
	if a == b {
		return true
	}
	diff := math.Abs(a - b)
	scale := math.Max(math.Abs(a), math.Abs(b))
	return diff/scale < 1e-15
}

func newTestClient(t *testing.T) *PricingExtensionClient {
	t.Helper()
	return &PricingExtensionClient{
		pluginName: "test",
		logger:     slog.Default(),
	}
}

func TestProtoToOverride_RoundTripsDecimalString(t *testing.T) {
	c := newTestClient(t)

	// 19-digit constants exceed float64's ~17-digit precision so any
	// regression to a `double` wire field would round these values.
	const rawInput = "0.1234567890123456789"
	const rawCache = "0.000000123456789"
	in := &pb.PricingOverride{
		Key: &pb.PricingOverrideKey{
			GroupId:  42,
			Platform: "ANTHROPIC",
			Model:    "claude-opus-4-7",
		},
		BillingMode:     "token",
		InputPrice:      rawInput,
		OutputPrice:     "0.000050",
		CacheWritePrice: rawCache,
		CacheReadPrice:  "", // unset → 0
		PerRequestPrice: "12345678.9012345678",
	}

	got := c.protoToOverride(in)
	if got.SourcePlugin != "test" {
		t.Errorf("SourcePlugin: got %q, want %q", got.SourcePlugin, "test")
	}
	if got.BillingMode != "token" {
		t.Errorf("BillingMode: got %q, want %q", got.BillingMode, "token")
	}

	checks := []struct {
		field string
		got   float64
		raw   string
	}{
		{"InputPrice", got.InputPrice, rawInput},
		{"OutputPrice", got.OutputPrice, "0.000050"},
		{"CacheWritePrice", got.CacheWritePrice, rawCache},
		{"PerRequestPrice", got.PerRequestPrice, "12345678.9012345678"},
	}
	for _, c := range checks {
		want := mustDecimalFloat(t, c.raw)
		if !approxEqual(c.got, want) {
			t.Errorf("%s: got %g, want ≈%g (raw %s)",
				c.field, c.got, want, c.raw)
		}
	}

	if got.CacheReadPrice != 0 {
		t.Errorf("CacheReadPrice: empty wire string should decode to 0, got %g",
			got.CacheReadPrice)
	}

	// Key normalisation: protoToOverride forwards the key verbatim; the
	// PricingOverrideCache is responsible for lower-casing on write.
	if got.Key.Platform != "ANTHROPIC" || got.Key.Model != "claude-opus-4-7" {
		t.Errorf("Key forwarded incorrectly: %+v", got.Key)
	}
}

func TestProtoToOverride_BadDecimalStringDecodesToZero(t *testing.T) {
	c := newTestClient(t)
	in := &pb.PricingOverride{
		Key: &pb.PricingOverrideKey{Platform: "openai", Model: "gpt-4"},
		// Garbage string — must not panic, must decode to 0 so the
		// gateway hot path keeps running.
		InputPrice: "not-a-decimal",
	}
	got := c.protoToOverride(in)
	if got.InputPrice != 0 {
		t.Errorf("bad decimal should decode to 0, got %g", got.InputPrice)
	}
}

func TestFormatFloatAsDecimalString(t *testing.T) {
	// T39: the host-side formatter is now decimalx.FormatFloatString. This
	// test stays here (rather than in plugin-sdk/decimalx) to assert that
	// the host's call sites produce the wire format the plugin expects.
	cases := []struct {
		name string
		in   float64
		want string
	}{
		{"zero-encodes-empty", 0, ""},
		{"simple-positive", 1.5, "1.5"},
		{"small-price", 0.000003, "0.000003"},
		{"negative", -0.5, "-0.5"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := decimalx.FormatFloatString(tc.in)
			if got != tc.want {
				t.Errorf("decimalx.FormatFloatString(%g) = %q, want %q",
					tc.in, got, tc.want)
			}
		})
	}
}

// TestProtoToOverride_IntervalRoundTrip locks in the same invariant for
// PricingInterval so a future encoder regression on the plugin side does
// not silently demote tier prices through float64.
func TestProtoToOverride_IntervalRoundTrip(t *testing.T) {
	c := newTestClient(t)
	in := &pb.PricingOverride{
		Key: &pb.PricingOverrideKey{Platform: "openai", Model: "gpt-5"},
		Intervals: []*pb.PricingInterval{
			{
				MinTokens:       0,
				MaxTokens:       8192,
				InputPrice:      "0.0000123456789012345",
				OutputPrice:     "0.000050",
				PerRequestPrice: "", // unset
			},
		},
	}
	got := c.protoToOverride(in)
	if len(got.Intervals) != 1 {
		t.Fatalf("intervals: got %d, want 1", len(got.Intervals))
	}
	iv := got.Intervals[0]
	if iv.MinTokens != 0 || iv.MaxTokens != 8192 {
		t.Errorf("token range: got [%d, %d), want [0, 8192)",
			iv.MinTokens, iv.MaxTokens)
	}
	wantInput := mustDecimalFloat(t, "0.0000123456789012345")
	if !approxEqual(iv.InputPrice, wantInput) {
		t.Errorf("InputPrice: got %g, want ≈%g", iv.InputPrice, wantInput)
	}
	if iv.PerRequestPrice != 0 {
		t.Errorf("PerRequestPrice: unset wire should decode to 0, got %g",
			iv.PerRequestPrice)
	}
}

func mustDecimalFloat(t *testing.T, raw string) float64 {
	t.Helper()
	d, err := decimal.NewFromString(raw)
	if err != nil {
		t.Fatalf("mustDecimalFloat(%q): %v", raw, err)
	}
	return d.InexactFloat64()
}
