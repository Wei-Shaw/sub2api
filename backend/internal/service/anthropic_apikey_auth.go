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

// isOllamaCloudAnthropicAuthBaseURL 判定本次实际选用的 Anthropic 上游 base_url
// 是否指向 Ollama Cloud。与真实出站 URL 组装同源：先 TrimSpace + TrimRight "/"
// （urlvalidator 与 nativeAnthropicTargetURL 同款归一化，'https://ollama.com/'
// 与 '.../v1' 均合理），再复用既有严格 host helper isOllamaCloudBaseURL（精确
// host、https、无 query/fragment、路径仅空或 /v1），不按 substring 匹配。
func isOllamaCloudAnthropicAuthBaseURL(baseURL string) bool {
	return isOllamaCloudBaseURL(strings.TrimRight(strings.TrimSpace(baseURL), "/"))
}

// setAnthropicAPIKeyAuthHeader 写入上游认证头。除 extra 覆写外，Ollama Cloud
// 官方 Anthropic 兼容端点只认 Authorization: Bearer：判定依据是本次实际选用的
// 上游 base_url（与出站 URL 组装同源，而非 account.Platform——Ollama Cloud key
// 可挂在多个平台分组下），即使 extra 缺失或显式 x_api_key 也强制 Bearer；
// 其它上游保持历史 extra/default 行为。baseURL 为该请求实际选用的 Anthropic
// 上游 base（GetBaseURL / GetAnthropicProtocolBaseURL 等），默认官方端点时传空。
func setAnthropicAPIKeyAuthHeader(header http.Header, account *Account, token, baseURL string) {
	if account.Type == AccountTypeAPIKey && isOllamaCloudAnthropicAuthBaseURL(baseURL) {
		header.Set("Authorization", "Bearer "+token)
		return
	}
	if account.GetAnthropicAPIKeyAuthScheme() == AnthropicAPIKeyAuthSchemeAuthorizationBearer {
		header.Set("Authorization", "Bearer "+token)
		return
	}
	header.Set("x-api-key", token)
}
