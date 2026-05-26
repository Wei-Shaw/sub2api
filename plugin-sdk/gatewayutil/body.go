package gatewayutil

import (
	"strings"
)

// MaxErrorBodyForProto caps the upstream error body embedded in the proto
// message to avoid excessive gRPC message sizes.
const MaxErrorBodyForProto = 8 << 10 // 8 KB

// TruncateErrorBodyLen is the maximum length of body text included in
// error messages during token refresh operations.
const TruncateErrorBodyLen = 200

// MaxResponseBodySize caps how much of an upstream HTTP response body is
// read into memory (both error responses and non-streaming success bodies).
// Shared across gateway plugins so the OOM-prevention bound stays consistent.
const MaxResponseBodySize = 10 << 20 // 10 MB

// TruncateBytes returns b truncated to at most maxLen bytes.
func TruncateBytes(b []byte, maxLen int) []byte {
	if len(b) <= maxLen {
		return b
	}
	return b[:maxLen]
}

// TruncateBody returns at most TruncateErrorBodyLen bytes of body text
// for error messages, appending "..." when truncated.
func TruncateBody(b []byte) string {
	if len(b) <= TruncateErrorBodyLen {
		return string(b)
	}
	return string(b[:TruncateErrorBodyLen]) + "..."
}

// MergeBetaHeaders merges two comma-separated beta header values,
// deduplicating entries. Used by anthropic, openai, and antigravity
// plugins to combine default and client-provided beta flags.
func MergeBetaHeaders(existing, additional string) string {
	if existing == "" {
		return additional
	}
	if additional == "" {
		return existing
	}
	seen := make(map[string]struct{})
	var parts []string
	for _, part := range strings.Split(existing, ",") {
		t := strings.TrimSpace(part)
		if t == "" {
			continue
		}
		if _, ok := seen[t]; !ok {
			seen[t] = struct{}{}
			parts = append(parts, t)
		}
	}
	for _, part := range strings.Split(additional, ",") {
		t := strings.TrimSpace(part)
		if t == "" {
			continue
		}
		if _, ok := seen[t]; !ok {
			seen[t] = struct{}{}
			parts = append(parts, t)
		}
	}
	return strings.Join(parts, ",")
}
