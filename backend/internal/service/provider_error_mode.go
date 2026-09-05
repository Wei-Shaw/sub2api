package service

import (
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

const (
	ProviderErrorModeProxy    = "proxy"
	ProviderErrorModeProvider = "provider"
)

func resolveProviderErrorMode(cfg *config.Config) string {
	if cfg == nil {
		return ProviderErrorModeProxy
	}
	if strings.EqualFold(strings.TrimSpace(cfg.Gateway.ProviderErrorMode), ProviderErrorModeProvider) {
		return ProviderErrorModeProvider
	}
	return ProviderErrorModeProxy
}

// normalizeProviderError changes only gateway-generated fallback errors. Real
// upstream error bodies and protocol events remain untouched, so provider
// details already supplied upstream are never erased by this helper.
func normalizeProviderError(mode, protocol string, status int, errType, message string) (string, string) {
	if !strings.EqualFold(strings.TrimSpace(mode), ProviderErrorModeProvider) {
		return errType, message
	}

	protocol = strings.ToLower(strings.TrimSpace(protocol))
	switch protocol {
	case "anthropic":
		switch status {
		case http.StatusUnauthorized:
			return "authentication_error", "Invalid authentication credentials"
		case http.StatusForbidden:
			return "permission_error", "Permission denied"
		case http.StatusTooManyRequests:
			return "rate_limit_error", "Rate limit exceeded"
		case 529:
			return "overloaded_error", "Overloaded"
		case http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
			return "api_error", "Internal server error"
		default:
			return errType, "Request failed"
		}
	case "openai", "codex":
		switch status {
		case http.StatusUnauthorized:
			return "invalid_request_error", "Incorrect API key provided."
		case http.StatusForbidden:
			return "permission_error", "You do not have access to this resource."
		case http.StatusTooManyRequests:
			return "rate_limit_error", "Rate limit reached for requests."
		case http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
			return "server_error", "The server had an error while processing your request."
		default:
			return errType, "Request failed"
		}
	default:
		return errType, message
	}
}
