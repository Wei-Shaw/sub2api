// Package decimalx centralises the float64 ↔ decimal.NullDecimal conversion
// helpers shared between the channel-management plugin's handler / service /
// repository / proto-server layers.
//
// The plugin's pricing fields moved from *float64 to decimal.NullDecimal in
// task T4 to satisfy the project rule "金额计算必须用 shopspring/decimal"
// (see CLAUDE.md "支付系统专项"). Three boundary translations stay here:
//
//   - REST JSON DTO and the vendored host domain snapshot still expect
//     float64 / *float64 (legacy contracts not in scope of T24).
//   - Validators / accumulators want NullDecimal-aware equivalents of the
//     legacy `p != nil && *p > 0` and `p != nil && *p` patterns.
//   - Tests want a one-liner price literal constructor.
//
// The PricingExtension proto wire codec (ToProtoString / FromProtoString /
// FromProtoStringOrZero / DecimalToProtoString / FormatFloatString /
// ParseFloatString) used to live here as well. T39 lifted those into the
// single-source `plugin-sdk/decimalx` package so the host and the plugin
// share one canonical implementation; call sites in this plugin now import
// `github.com/Wei-Shaw/sub2api/plugin-sdk/decimalx` directly.
package decimalx

import (
	"github.com/shopspring/decimal"
)

// FromFloat64Ptr converts a *float64 (the pre-T4 pricing shape) into a
// decimal.NullDecimal. nil maps to "unset" (Valid=false). Any non-nil
// value, even 0, becomes a valid decimal so callers can distinguish
// "explicitly zero" from "not provided" later if needed.
func FromFloat64Ptr(p *float64) decimal.NullDecimal {
	if p == nil {
		return decimal.NullDecimal{}
	}
	return decimal.NullDecimal{Decimal: decimal.NewFromFloat(*p), Valid: true}
}

// ToFloat64Ptr is the inverse of FromFloat64Ptr. Invalid (unset) values
// return nil; valid values are converted via decimal.InexactFloat64. The
// loss is acceptable at boundaries that contractually use float64 (proto
// double, vendored host *float64, REST DTO number) — the canonical
// representation lives inside service / repo as decimal.NullDecimal.
func ToFloat64Ptr(d decimal.NullDecimal) *float64 {
	if !d.Valid {
		return nil
	}
	v := d.Decimal.InexactFloat64()
	return &v
}

// ToFloat64 returns the float64 form of d, treating "unset" as 0. Used by
// proto encoding paths where the wire zero already means "not provided".
func ToFloat64(d decimal.NullDecimal) float64 {
	if !d.Valid {
		return 0
	}
	return d.Decimal.InexactFloat64()
}

// IsPositive reports whether d is set and strictly greater than zero.
// Mirrors the legacy `p != nil && *p > 0` idiom used across the pricing
// validators.
func IsPositive(d decimal.NullDecimal) bool {
	if !d.Valid {
		return false
	}
	return d.Decimal.Sign() > 0
}

// IsNegative reports whether d is set and strictly less than zero.
// Mirrors the legacy `p != nil && *p < 0` idiom used in price validators.
func IsNegative(d decimal.NullDecimal) bool {
	if !d.Valid {
		return false
	}
	return d.Decimal.Sign() < 0
}

// OrZero returns d's underlying decimal, or zero when d is unset. Useful
// for accumulator paths that need to keep adding regardless of which
// price components were configured.
func OrZero(d decimal.NullDecimal) decimal.Decimal {
	if !d.Valid {
		return decimal.Zero
	}
	return d.Decimal
}

// MustPrice is the test-friendly constructor that panics on parse failure.
// Production code should use decimal.NewFromString or
// decimal.RequireFromString directly; MustPrice exists so unit tests can
// declare prices in a single line:
//
//	pricing := MustPrice("0.001")
//
// without having to handle the error path that never triggers for static
// literals.
func MustPrice(s string) decimal.NullDecimal {
	v, err := decimal.NewFromString(s)
	if err != nil {
		panic("decimalx.MustPrice: invalid decimal literal " + s + ": " + err.Error())
	}
	return decimal.NullDecimal{Decimal: v, Valid: true}
}
