// Package decimalx is the single-source codec for the PricingExtension proto
// wire format (T24 + T39).
//
// Before T39, host (`backend/internal/plugin/pricing_extension_client.go`) and
// plugin (`plugins/channel-management/internal/decimalx`) each implemented
// their own decimal-string encoder / decoder. Two parallel implementations of
// the same wire contract violate the CLAUDE.md "复用 (Reuse)" principle —
// changes to the proto edge had to be applied twice and could drift.
//
// This package owns:
//
//   - ToProtoString / FromProtoString — the canonical NullDecimal <-> wire
//     pair. Used on plugin Watch encode + host Watch decode + Pricing /
//     ResolveAccountStatsCost RPCs in both directions.
//   - FromProtoStringOrZero — best-effort decode for paths that must not
//     fail the gateway hot path on a single malformed payload.
//   - DecimalToProtoString — the no-nullability variant for cost outputs
//     where "0" is a meaningful value.
//   - FormatFloatString — host-side float64 -> wire encoder. Float64 input
//     is unavoidable at boundaries that still hold legacy float64 (the host
//     BillingService computes cost as float64); a single channel that
//     funnels through here is preferable to ad-hoc decimal.NewFromFloat
//     calls scattered across the host.
//   - ParseFloatString — host-side wire -> float64 decoder, the inverse of
//     FormatFloatString. Parse failures degrade to 0 with no error so the
//     hot path never aborts on a single malformed plugin payload.
//
// Plugins that still need NullDecimal-friendly Float helpers (FromFloat64Ptr
// / ToFloat64Ptr / IsPositive / IsNegative / OrZero / MustPrice) keep them
// in their own internal packages — those are NullDecimal value helpers, not
// proto-wire concerns. Only the wire codec is centralised here.
package decimalx

import (
	"fmt"

	"github.com/shopspring/decimal"
)

// ToProtoString encodes a decimal.NullDecimal for the PricingExtension proto
// wire. Invalid (unset) values map to the empty string ("") which the
// receiving side treats as "not set". Valid values flow through
// decimal.Decimal.String() so the canonical text representation (with
// trailing zeros stripped) round-trips losslessly through ParseString.
func ToProtoString(d decimal.NullDecimal) string {
	if !d.Valid {
		return ""
	}
	return d.Decimal.String()
}

// FromProtoString is the inverse of ToProtoString. The empty string maps to
// decimal.NullDecimal{Valid:false} ("not set") so legacy zero/unset semantics
// survive the migration. Any non-empty input is parsed via
// decimal.NewFromString; parse errors are surfaced to the caller so they can
// decide whether to fall back to a default or fail the RPC.
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

// FromProtoStringOrZero maps parse failures to decimal.Zero so callers on
// the proto-decode path can keep the gateway hot path running. The error is
// intentionally swallowed; callers who want to surface parse failures
// should use FromProtoString directly.
func FromProtoStringOrZero(s string) decimal.Decimal {
	d, err := FromProtoString(s)
	if err != nil || !d.Valid {
		return decimal.Zero
	}
	return d.Decimal
}

// DecimalToProtoString encodes a plain decimal.Decimal (no nullability) for
// the proto wire. The zero value is encoded as "0", not "" — callers that
// want "0 means unset" semantics should pass a NullDecimal through
// ToProtoString instead.
func DecimalToProtoString(d decimal.Decimal) string {
	return d.String()
}

// FormatFloatString encodes a host-side float64 price as the canonical
// decimal-string for the proto wire. Zero maps to the empty string ("not
// set") to match the legacy zero-as-unset semantics; any other value goes
// through decimal.NewFromFloat -> String() so the wire representation stays
// readable and round-trips through FromProtoString without trailing nines.
//
// The IEEE-754 noise riding on float64 inputs ends up in the proto payload
// here — that is acceptable because the inputs are themselves host-computed
// float64 (BillingService.computeCost). Migrating the host pipeline to
// decimal.Decimal end-to-end is a much larger change tracked separately.
func FormatFloatString(v float64) string {
	if v == 0 {
		return ""
	}
	return decimal.NewFromFloat(v).String()
}

// ParseFloatString parses a proto decimal string into float64. Empty string
// maps to 0 (the cache treats zero as "unset"). A parse failure degrades to
// 0 so the gateway hot path never aborts on a single malformed plugin
// payload; callers who care about the error path should use FromProtoString
// instead.
//
// The host side still consumes float64 because BillingService / channel
// pricing layers are float64-based. The single bounded loss happens at this
// boundary only — proto wire and plugin both speak canonical decimal, and
// the host cache lives behind a 12-digit NUMERIC(20,12) DB schema, so the
// 53-bit IEEE-754 mantissa still covers every price magnitude in production.
func ParseFloatString(s string) float64 {
	if s == "" {
		return 0
	}
	d, err := decimal.NewFromString(s)
	if err != nil {
		return 0
	}
	return d.InexactFloat64()
}
