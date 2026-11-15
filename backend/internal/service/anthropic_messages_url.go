package service

import (
	"net/url"
	"strings"
)

// buildAnthropicMessagesURL appends the Messages endpoint to an already validated
// base URL. Cloudflare's account REST endpoint rejects the beta query parameter;
// all other upstreams, including Cloudflare's native gateway, keep it.
func buildAnthropicMessagesURL(baseURL string) string {
	targetURL := strings.TrimRight(baseURL, "/") + "/v1/messages"
	parsed, err := url.Parse(targetURL)
	if err == nil && strings.EqualFold(parsed.Scheme, "https") && parsed.User == nil &&
		parsed.RawQuery == "" && !parsed.ForceQuery && parsed.Fragment == "" && parsed.RawPath == "" {
		host := strings.ToLower(parsed.Host)
		parts := strings.Split(parsed.Path, "/")
		if (host == "api.cloudflare.com" || host == "api.cloudflare.com:443") &&
			len(parts) == 8 && parts[0] == "" && parts[1] == "client" && parts[2] == "v4" &&
			parts[3] == "accounts" && parts[4] != "" && parts[4] != "." && parts[4] != ".." &&
			parts[5] == "ai" && parts[6] == "v1" && parts[7] == "messages" {
			return targetURL
		}
	}
	return targetURL + "?beta=true"
}
