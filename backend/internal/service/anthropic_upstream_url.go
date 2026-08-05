package service

import "strings"

// buildAnthropicUpstreamURL appends an Anthropic endpoint path without
// duplicating the /v1 segment when the configured base URL already includes it.
func buildAnthropicUpstreamURL(baseURL, path string) string {
	trimmedBaseURL := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if strings.HasSuffix(trimmedBaseURL, "/v1") && strings.HasPrefix(path, "/v1/") {
		path = strings.TrimPrefix(path, "/v1")
	}
	return trimmedBaseURL + path + "?beta=true"
}
