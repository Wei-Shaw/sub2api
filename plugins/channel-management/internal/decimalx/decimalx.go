// Package decimalx centralises the float64 ↔ decimal.NullDecimal conversion
// helpers shared between the channel-management plugin's handler / service /
// repository / proto-server layers.
//
// The plugin's pricing fields moved from *float64 to decimal.NullDecimal in
// task T4 to satisfy the project rule "金额计算必须用 shopspring/decimal"
// (see CLAUDE.md "支付系统专项"). External boundaries fall into two camps:
//
//   - REST JSON DTO and the vendored host domain snapshot still expect
//     float64 / *float64 (legacy contracts not in scope of T24).
//   - The PricingExtension proto edge moved to string decimal in T24
//     (plugin-sdk/proto/sdk.proto). ToProtoString / FromProtoString below
//     are the authoritative encoders for that hop.
//
// The helpers here keep all four boundary translations in one place so the
// rest of the plugin can compose a single import.
package decimalx

import (
	"fmt"

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

// ToProtoString encodes a decimal.NullDecimal for the PricingExtension
// proto wire (T24). Invalid (unset) values map to the empty string ""
// which the receiving side treats as "not set". Valid values are emitted
// via decimal.Decimal.String() so the canonical text representation
// (with trailing zeros stripped) round-trips losslessly through
// decimal.NewFromString on the host.
//
// This is the authoritative encoder for plugin → host pricing values; do
// not call decimal.Decimal.InexactFloat64 at the proto edge.
func ToProtoString(d decimal.NullDecimal) string {
	if !d.Valid {
		return ""
	}
	return d.Decimal.String()
}

// FromProtoString is the inverse of ToProtoString. The empty string maps
// to decimal.NullDecimal{Valid:false} ("not set") so legacy zero/unset
// semantics survive the migration. Any non-empty input is parsed via
// decimal.NewFromString; parse errors are surfaced to the caller so they
// can decide whether to fall back to a default or fail the RPC.
//
// Used by both plugin-side decoding (Watch stream events that echo back
// our own encodings) and host-side decoding of plugin output.
func FromProtoString(s string) (decimal.NullDecimal, error) {
	if s == "" {
		return decimal.NullDecimal{}, nil
	}
	v, err := decimal.NewFromString(s)
	if err != nil {
		return decimal.NullDecimal{}, fmt.Errorf("decimalx.FromProtoString %q: %w", s, err)
	}
	return decimal.NullDecimal{Decimal: v, Valid: true}, nil
}

// FromProtoStringOrZero is a convenience wrapper that maps parse failures
// to decimal.Zero so callers on the proto-decode path can keep the
// gateway hot path running. The error is intentionally swallowed; callers
// who want to surface parse failures should use FromProtoString directly.
func FromProtoStringOrZero(s string) decimal.Decimal {
	d, err := FromProtoString(s)
	if err != nil || !d.Valid {
		return decimal.Zero
	}
	return d.Decimal
}

// DecimalToProtoString encodes a plain decimal.Decimal (no nullability)
// for the proto wire. The zero value is encoded as "0", not "" — callers
// that want "0 means unset" semantics should pass a NullDecimal through
// ToProtoString instead.
func DecimalToProtoString(d decimal.Decimal) string {
	return d.String()
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
