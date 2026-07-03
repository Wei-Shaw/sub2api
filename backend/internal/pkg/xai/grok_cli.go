package xai

import (
	"net/http"
	"strings"
)

const (
	DefaultAPIBaseURL   = "https://api.x.ai/v1"
	CLIChatProxyBaseURL = "https://cli-chat-proxy.grok.com/v1"
)

func IsGrokCLIClient(userAgent string) bool {
	ua := strings.ToLower(strings.TrimSpace(userAgent))
	return strings.Contains(ua, "xai-grok-cli")
}

func HasGrokCLIRequestHeader(headers http.Header) bool {
	if headers == nil {
		return false
	}
	return strings.TrimSpace(headers.Get("x-grok-client")) != "" ||
		strings.TrimSpace(headers.Get("x-grok-conv-id")) != ""
}

func IsGrokCLIHTTPRequest(userAgent, model string, headers http.Header) bool {
	return IsGrokCLIClient(userAgent) || IsGrokCLIModel(model) || HasGrokCLIRequestHeader(headers)
}

func IsGrokCLIModel(model string) bool {
	model = strings.TrimSpace(model)
	if model == "" {
		return false
	}
	if model == "grok-build" {
		return true
	}
	return strings.HasPrefix(model, "grok-composer-")
}

func IsCLIChatProxyBaseURL(baseURL string) bool {
	base := strings.ToLower(strings.TrimRight(strings.TrimSpace(baseURL), "/"))
	if base == "" {
		return false
	}
	return strings.Contains(base, "cli-chat-proxy.grok.com")
}

func ResolveInferenceBaseURL(accountBaseURL string) string {
	base := strings.TrimRight(strings.TrimSpace(accountBaseURL), "/")
	if base == "" {
		base = DefaultAPIBaseURL
	}
	return base
}

func ShouldUseGrokCLINormalize(accountBaseURL, userAgent, model string, headers http.Header) bool {
	if IsCLIChatProxyBaseURL(accountBaseURL) {
		return true
	}
	return IsGrokCLIHTTPRequest(userAgent, model, headers)
}

func BuildModelsV2URL(baseURL string) string {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" {
		base = CLIChatProxyBaseURL
	}
	if strings.HasSuffix(base, "/models-v2") {
		return base
	}
	base = strings.TrimSuffix(base, "/responses")
	base = strings.TrimSuffix(base, "/models")
	return base + "/models-v2"
}

func ForwardGrokCLIRequestHeaders(dst http.Header, src http.Header, promptCacheKey string) {
	if dst == nil || src == nil {
		return
	}
	for key, values := range src {
		lowerKey := strings.ToLower(strings.TrimSpace(key))
		switch {
		case lowerKey == "accept-language", lowerKey == "user-agent":
			for _, value := range values {
				dst.Add(key, value)
			}
		case strings.HasPrefix(lowerKey, "x-grok-"):
			if lowerKey == "x-grok-conv-id" && strings.TrimSpace(promptCacheKey) != "" {
				continue
			}
			for _, value := range values {
				dst.Add(key, value)
			}
		}
	}
	if strings.TrimSpace(promptCacheKey) != "" {
		dst.Set("x-grok-conv-id", strings.TrimSpace(promptCacheKey))
	}
}
