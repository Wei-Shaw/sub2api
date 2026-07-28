package service

import (
	"regexp"
	"strings"
)

var (
	providerCorrelationIDDelimitedRE = regexp.MustCompile(`(?i)\s*[\(\[]\s*(?:request|trace|correlation)[\s_-]*id\s*[:=]\s*[A-Za-z0-9_.:/+=-]{6,}\s*[\)\]]`)
	providerCorrelationIDSuffixRE    = regexp.MustCompile(`(?i)\s+(?:request|trace|correlation)[\s_-]*id\s*[:=]\s*[[:alnum:]_.:/-]{6,}\s*$`)
)

// ExtractClientSafeUpstreamErrorMessage keeps actionable provider diagnostics
// while removing provider-only correlation IDs from the client response.
// Raw upstream bodies and IDs remain available in ops error context.
func ExtractClientSafeUpstreamErrorMessage(body []byte) string {
	return sanitizeClientUpstreamErrorMessage(extractUpstreamErrorMessage(body))
}

func sanitizeClientUpstreamErrorMessage(message string) string {
	message = sanitizeUpstreamErrorMessage(strings.TrimSpace(message))
	message = providerCorrelationIDDelimitedRE.ReplaceAllString(message, "")
	message = providerCorrelationIDSuffixRE.ReplaceAllString(message, "")
	return strings.TrimSpace(message)
}
