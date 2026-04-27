package plugin

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
)

// W3C traceparent header has the layout:
//
//	version "-" trace-id "-" parent-id "-" trace-flags
//
// version is 2 hex chars, trace-id is 32 hex chars (16 bytes), parent-id is
// 16 hex chars (8 bytes), trace-flags is 2 hex chars. Total length is always
// 55 chars including the 3 dashes. We support only version "00".
//
// Reference: https://www.w3.org/TR/trace-context/#traceparent-header
const (
	traceparentSupportedVersion = "00"
	traceparentLength           = 55 // 2 + 1 + 32 + 1 + 16 + 1 + 2
	traceIDHexLen               = 32
	spanIDHexLen                = 16
)

// invalidTraceID and invalidSpanID are the zero ids defined by W3C; values
// equal to them MUST be rejected as invalid.
var (
	invalidTraceID = strings.Repeat("0", traceIDHexLen)
	invalidSpanID  = strings.Repeat("0", spanIDHexLen)
)

// isValidTraceparent reports whether tp matches the W3C traceparent format
// for version 00 and contains non-zero trace/span ids.
//
// Anything else (wrong length, unknown version, all-zero ids, non-hex chars)
// is treated as invalid; callers should generate a fresh traceparent rather
// than propagate a malformed value to plugins.
func isValidTraceparent(tp string) bool {
	if len(tp) != traceparentLength {
		return false
	}
	parts := strings.Split(tp, "-")
	if len(parts) != 4 {
		return false
	}
	version, traceID, spanID, flags := parts[0], parts[1], parts[2], parts[3]
	if version != traceparentSupportedVersion {
		return false
	}
	if len(traceID) != traceIDHexLen || len(spanID) != spanIDHexLen || len(flags) != 2 {
		return false
	}
	if traceID == invalidTraceID || spanID == invalidSpanID {
		return false
	}
	if !isLowerHex(traceID) || !isLowerHex(spanID) || !isLowerHex(flags) {
		return false
	}
	return true
}

// newTraceparent creates a fresh W3C traceparent. trace-flags is fixed to 01
// (sampled=true) — we ask plugins to log/trace everything, sampling decisions
// happen at the host level.
//
// Returns "" only if crypto/rand fails, which is unrecoverable; callers
// already treat empty as "no traceparent" and skip propagation.
func newTraceparent() string {
	var traceID [16]byte
	var spanID [8]byte
	if _, err := rand.Read(traceID[:]); err != nil {
		return ""
	}
	if _, err := rand.Read(spanID[:]); err != nil {
		return ""
	}
	return traceparentSupportedVersion + "-" +
		hex.EncodeToString(traceID[:]) + "-" +
		hex.EncodeToString(spanID[:]) + "-01"
}

func isLowerHex(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= '0' && c <= '9':
		case c >= 'a' && c <= 'f':
		default:
			return false
		}
	}
	return true
}
