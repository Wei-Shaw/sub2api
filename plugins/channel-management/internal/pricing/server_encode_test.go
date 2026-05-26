// server_encode_test.go — coverage for encodeOverride / encodeIntervals,
// the plugin-side wire encoders that turn ChannelModelPricing rows into
// pluginsdk.PricingOverride messages.
//
// T24 migrated every price field from `double` to a string decimal. The
// invariant we lock in here is the most important one in the migration:
// a ChannelModelPricing whose decimal.NullDecimal carries 19+ significant
// digits emits a proto string that exactly matches decimal.Decimal.String()
// — no IEEE-754 rounding sneaks in at the wire.
//
// We deliberately keep the host-side decoding test out of this file: the
// host parses via shopspring/decimal directly inside protoToOverride, and
// internal/decimalx already exercises the matching FromProtoString
// invariants.

package pricing

import (
	"testing"

	sdkdecimalx "github.com/Wei-Shaw/sub2api/plugin-sdk/decimalx"
	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
	"github.com/Wei-Shaw/sub2api/plugins/channel-management/internal/decimalx"
	chService "github.com/Wei-Shaw/sub2api/plugins/channel-management/service"
	"github.com/shopspring/decimal"
)

func TestEncodeOverride_RoundTripDecimalString(t *testing.T) {
	// 19-digit value chosen so float64 cannot represent it exactly. If
	// the encoder ever regresses to ToFloat64, the InexactFloat64 hop
	// will round these digits and the test fails immediately.
	// Each constant has 19 significant digits with no trailing zero so
	// decimal.Decimal.String()'s canonicalisation does not strip any
	// digits. The first two exceed float64's ~17-digit mantissa so a
	// regression to InexactFloat64 would round them visibly.
	const rawInput = "0.1234567890123456789"
	const rawOutput = "9.876543210987654321"
	const rawCacheWrite = "0.0000000123456789"
	const rawPerReq = "12345678.9012345678"

	p := &chService.ChannelModelPricing{
		Platform:        chService.PlatformAnthropic,
		Models:          []string{"test-model"},
		BillingMode:     chService.BillingModeToken,
		InputPrice:      decimalx.MustPrice(rawInput),
		OutputPrice:     decimalx.MustPrice(rawOutput),
		CacheWritePrice: decimalx.MustPrice(rawCacheWrite),
		// CacheReadPrice intentionally left invalid so we exercise the
		// "" wire encoding for unset values.
		PerRequestPrice: decimalx.MustPrice(rawPerReq),
	}

	got := encodeOverride(7, "anthropic", "test-model", p)

	checks := []struct {
		field string
		got   string
		want  string
	}{
		{"input_price", got.GetInputPrice(), rawInput},
		{"output_price", got.GetOutputPrice(), rawOutput},
		{"cache_write_price", got.GetCacheWritePrice(), rawCacheWrite},
		{"cache_read_price", got.GetCacheReadPrice(), ""}, // unset → ""
		{"per_request_price", got.GetPerRequestPrice(), rawPerReq},
		{"image_output_price", got.GetImageOutputPrice(), ""}, // unset → ""
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s: got %q, want %q", c.field, c.got, c.want)
		}
	}

	// Round-trip: parse each non-empty wire value back through the
	// shared decimalx helper and assert it equals the original decimal.
	parseChecks := []struct {
		field string
		wire  string
		want  decimal.NullDecimal
	}{
		{"input_price", got.GetInputPrice(), p.InputPrice},
		{"output_price", got.GetOutputPrice(), p.OutputPrice},
		{"cache_write_price", got.GetCacheWritePrice(), p.CacheWritePrice},
		{"per_request_price", got.GetPerRequestPrice(), p.PerRequestPrice},
	}
	for _, c := range parseChecks {
		back, err := sdkdecimalx.FromProtoString(c.wire)
		if err != nil {
			t.Fatalf("%s: FromProtoString(%q): %v", c.field, c.wire, err)
		}
		if !back.Valid {
			t.Errorf("%s: expected Valid=true after round-trip", c.field)
			continue
		}
		if !back.Decimal.Equal(c.want.Decimal) {
			t.Errorf("%s: round-trip changed value: in=%s wire=%s back=%s",
				c.field, c.want.Decimal.String(), c.wire, back.Decimal.String())
		}
	}
}

func TestEncodeOverride_UnsetPricesEncodeEmpty(t *testing.T) {
	p := &chService.ChannelModelPricing{
		Platform:    chService.PlatformOpenAI,
		Models:      []string{"unset-model"},
		BillingMode: chService.BillingModeToken,
		// All prices left at zero-value decimal.NullDecimal{} (Valid=false).
	}

	got := encodeOverride(0, "openai", "unset-model", p)
	emptyFields := map[string]string{
		"input_price":        got.GetInputPrice(),
		"output_price":       got.GetOutputPrice(),
		"cache_write_price":  got.GetCacheWritePrice(),
		"cache_read_price":   got.GetCacheReadPrice(),
		"image_output_price": got.GetImageOutputPrice(),
		"per_request_price":  got.GetPerRequestPrice(),
	}
	for name, got := range emptyFields {
		if got != "" {
			t.Errorf("%s: got %q, want empty string for unset price", name, got)
		}
	}
}

func TestEncodeIntervals_StringEncoding(t *testing.T) {
	maxTokens := 8192
	in := []chService.PricingInterval{
		{
			MinTokens:        0,
			MaxTokens:        &maxTokens,
			InputPrice:       decimalx.MustPrice("0.0000123456789012345"),
			OutputPrice:      decimalx.MustPrice("0.000050"),
			ImageOutputPrice: decimalx.MustPrice("0.04"),
			PerRequestPrice:  decimal.NullDecimal{}, // unset
		},
	}
	got := encodeIntervals(in)
	if len(got) != 1 {
		t.Fatalf("encodeIntervals: got %d entries, want 1", len(got))
	}
	iv := got[0]
	if iv.GetInputPrice() != "0.0000123456789012345" {
		t.Errorf("input_price: got %q, want %q",
			iv.GetInputPrice(), "0.0000123456789012345")
	}
	if iv.GetOutputPrice() != "0.00005" {
		// decimal.Decimal.String() strips trailing zeroes, so "0.000050"
		// canonicalises to "0.00005". This is part of the contract.
		t.Errorf("output_price: got %q, want %q (canonical form of 0.000050)",
			iv.GetOutputPrice(), "0.00005")
	}
	// T29: ImageOutputPrice now flows from service struct → proto, instead
	// of being hard-coded to "". Lock this in so a regression that drops
	// the field assignment fails immediately.
	if iv.GetImageOutputPrice() != "0.04" {
		t.Errorf("image_output_price: got %q, want %q",
			iv.GetImageOutputPrice(), "0.04")
	}
	if iv.GetPerRequestPrice() != "" {
		t.Errorf("per_request_price: got %q, want empty for unset",
			iv.GetPerRequestPrice())
	}
	if iv.GetMaxTokens() != int64(maxTokens) {
		t.Errorf("max_tokens: got %d, want %d", iv.GetMaxTokens(), maxTokens)
	}
}

// TestEncodeIntervals_UnsetImageOutputPriceEncodesEmpty locks in the
// "unset → empty string" wire contract for image_output_price specifically.
// Before T29 the encoder hard-coded "" regardless of the input; this test
// guards the new behaviour: empty when unset, populated when set.
func TestEncodeIntervals_UnsetImageOutputPriceEncodesEmpty(t *testing.T) {
	in := []chService.PricingInterval{
		{
			MinTokens:  0,
			InputPrice: decimalx.MustPrice("0.001"),
			// ImageOutputPrice intentionally left unset.
		},
	}
	got := encodeIntervals(in)
	if len(got) != 1 {
		t.Fatalf("encodeIntervals: got %d entries, want 1", len(got))
	}
	if v := got[0].GetImageOutputPrice(); v != "" {
		t.Errorf("image_output_price: got %q, want empty string for unset", v)
	}
}

// TestPricingOverride_NoFloatFieldsRemain is a structural belt-and-braces
// test: it instantiates the proto type and verifies every price-bearing
// accessor returns string. If a future refactor accidentally re-introduces
// `double` fields, this test fails to compile (signature changes) instead
// of silently regressing precision in production.
func TestPricingOverride_NoFloatFieldsRemain(t *testing.T) {
	o := &pb.PricingOverride{}
	checks := []func() string{
		o.GetInputPrice,
		o.GetOutputPrice,
		o.GetCacheWritePrice,
		o.GetCacheReadPrice,
		o.GetImageOutputPrice,
		o.GetPerRequestPrice,
	}
	for i, fn := range checks {
		if fn() != "" {
			t.Errorf("override field %d: zero-value should be empty string", i)
		}
	}

	iv := &pb.PricingInterval{}
	ivChecks := []func() string{
		iv.GetInputPrice,
		iv.GetOutputPrice,
		iv.GetCacheWritePrice,
		iv.GetCacheReadPrice,
		iv.GetImageOutputPrice,
		iv.GetPerRequestPrice,
	}
	for i, fn := range ivChecks {
		if fn() != "" {
			t.Errorf("interval field %d: zero-value should be empty string", i)
		}
	}
}
