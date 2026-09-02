package service

import (
	"net/http"
	"strings"
)

const (
	anthropicAPIKeyAuthSchemeExtraKey = "anthropic_apikey_auth_scheme"

	AnthropicAPIKeyAuthSchemeXAPIKey             = "x_api_key"
	AnthropicAPIKeyAuthSchemeAuthorizationBearer = "authorization_bearer"
)

// GetAnthropicAPIKeyAuthScheme returns the upstream authentication scheme for
// Anthropic API-key accounts. Missing or invalid values keep the historical
// x-api-key behavior. CN providers using their native Anthropic endpoints
// (api_protocol=anthropic) share the same override knob — Kimi/DeepSeek default
// to x-api-key, Zhipu can opt into Authorization: Bearer.
func (a *Account) GetAnthropicAPIKeyAuthScheme() string {
	if a == nil || a.Type != AccountTypeAPIKey {
		return AnthropicAPIKeyAuthSchemeXAPIKey
	}
	if a.Platform != PlatformAnthropic && !a.IsCNProvider() {
		return AnthropicAPIKeyAuthSchemeXAPIKey
	}

	switch strings.TrimSpace(a.GetExtraString(anthropicAPIKeyAuthSchemeExtraKey)) {
	case AnthropicAPIKeyAuthSchemeAuthorizationBearer:
		return AnthropicAPIKeyAuthSchemeAuthorizationBearer
	default:
		return AnthropicAPIKeyAuthSchemeXAPIKey
	}
}

func setAnthropicAPIKeyAuthHeader(header http.Header, account *Account, token string) {
	if account.GetAnthropicAPIKeyAuthScheme() == AnthropicAPIKeyAuthSchemeAuthorizationBearer {
		header.Set("Authorization", "Bearer "+token)
		return
	}
	header.Set("x-api-key", token)
}
