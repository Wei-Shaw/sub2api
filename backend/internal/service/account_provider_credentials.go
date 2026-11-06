package service

import (
	"encoding/json"
	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"net/url"
	"strconv"
	"strings"
)

func (a *Account) GetOpenAIAccessToken() string {
	if !a.IsOpenAI() {
		return ""
	}
	return a.GetCredential("access_token")
}

func (a *Account) GetOpenAIRefreshToken() string {
	if !a.IsOpenAIOAuth() {
		return ""
	}
	return a.GetCredential("refresh_token")
}

// GetGrokBaseURL selects the upstream used by Grok text and Responses traffic.
// Grok media traffic has a different transport contract and must use
// GetGrokMediaBaseURL instead.
//
// The stored base_url only rewrites forwarding endpoints. Credential lifecycle
// traffic (OAuth authorization and token refresh) always uses the official
// auth endpoints regardless of this value.
func (a *Account) GetGrokBaseURL() string {
	if a == nil || !a.IsGrok() {
		return ""
	}
	if a.IsGrokOAuth() {
		return a.GetGrokBaseURLOr(xai.DefaultCLIBaseURL)
	}
	return a.GetGrokBaseURLOr(xai.DefaultBaseURL)
}

// GetGrokBaseURLOr resolves an explicit account endpoint, falling back to the
// supplied default. Official OAuth endpoints are normalized here; custom
// endpoints are retained for the request builder's operator URL policy.
func (a *Account) GetGrokBaseURLOr(defaultBaseURL string) string {
	if a == nil || !a.IsGrok() {
		return ""
	}
	defaultBaseURL = strings.TrimRight(strings.TrimSpace(defaultBaseURL), "/")
	if defaultBaseURL == "" {
		if a.IsGrokOAuth() {
			defaultBaseURL = xai.DefaultCLIBaseURL
		} else {
			defaultBaseURL = xai.DefaultBaseURL
		}
	}
	baseURL := strings.TrimSpace(a.GetCredential("base_url"))
	if baseURL == "" {
		return defaultBaseURL
	}
	if !a.IsGrokOAuth() {
		return baseURL
	}
	// Explicit regional/API or custom values remain pinned. Custom endpoints are checked by the
	// operator URL policy at the request builder, which has access to config.
	if validated, err := xai.ValidateTrustedBaseURL(baseURL); err == nil {
		return validated
	}
	if parsed, err := url.Parse(baseURL); err == nil && parsed.Scheme != "" && parsed.Host != "" &&
		parsed.User == nil && parsed.RawQuery == "" && parsed.Fragment == "" {
		return strings.TrimRight(baseURL, "/")
	}
	return defaultBaseURL
}

// GetGrokMediaBaseURL selects the upstream used by Grok Imagine APIs.
// The subscription CLI gateway enforces a small request-body limit that
// rejects large Base64 media payloads, so OAuth media leaves for api.x.ai
// whenever text traffic resolves to the CLI gateway. Every other manually
// selected endpoint (official/regional API hosts or custom relays) serves
// media as-is.
func (a *Account) GetGrokMediaBaseURL() string {
	if !a.IsGrok() {
		return ""
	}
	baseURL := a.GetGrokBaseURL()
	if a.IsGrokOAuth() && isGrokCLIProxyTarget(baseURL) {
		return xai.DefaultBaseURL
	}
	return baseURL
}

func (a *Account) GetGrokAccessToken() string {
	if !a.IsGrok() {
		return ""
	}
	return a.GetCredential("access_token")
}

func (a *Account) GetGrokRefreshToken() string {
	if !a.IsGrokOAuth() {
		return ""
	}
	return a.GetCredential("refresh_token")
}

func (a *Account) GetOpenAIIDToken() string {
	if !a.IsOpenAIOAuth() {
		return ""
	}
	return a.GetCredential("id_token")
}

func (a *Account) GetOpenAIApiKey() string {
	if !a.IsOpenAIApiKey() {
		return ""
	}
	return a.GetCredential("api_key")
}

// GetOpenAIProtocolAPIKey 返回 OpenAI 协议族 APIKey 账号的密钥。
// 覆盖 openai 原生账号、国产 OpenAI 兼容供应商（kimi/zhipu/deepseek）
// 以及 OpenCode Go 账号，供转发鉴权、模型列表同步等协议族共用路径使用。
// 注意 IsOpenAIApiKey 语义上仅指 openai 平台账号，调度倍率/WS 能力门控
// 继续以其为准，不受本方法影响。
func (a *Account) GetOpenAIProtocolAPIKey() string {
	if a == nil {
		return ""
	}
	if a.IsMultiProtocolAPIKey() {
		if a.Type != AccountTypeAPIKey {
			return ""
		}
		return a.GetCredential("api_key")
	}
	return a.GetOpenAIApiKey()
}

func (a *Account) GetOpenAIUserAgent() string {
	if !a.IsOpenAI() {
		return ""
	}
	return a.GetCredential("user_agent")
}

func (a *Account) GetChatGPTAccountID() string {
	if !a.IsOpenAIOAuthLike() {
		return ""
	}
	return a.GetCredential("chatgpt_account_id")
}

func (a *Account) IsChatGPTAccountFedRAMP() bool {
	if !a.IsOpenAIOAuthLike() || a.Credentials == nil {
		return false
	}
	v, ok := a.Credentials["chatgpt_account_is_fedramp"]
	if !ok || v == nil {
		return false
	}
	switch value := v.(type) {
	case bool:
		return value
	case string:
		parsed, err := strconv.ParseBool(strings.TrimSpace(value))
		return err == nil && parsed
	case json.Number:
		parsed, err := strconv.ParseBool(value.String())
		return err == nil && parsed
	case float64:
		return value != 0
	case int:
		return value != 0
	case int64:
		return value != 0
	default:
		return false
	}
}

func (a *Account) GetOpenAIDeviceID() string {
	if !a.IsOpenAIOAuth() {
		return ""
	}
	return strings.TrimSpace(a.GetExtraString("openai_device_id"))
}

func (a *Account) GetOpenAISessionID() string {
	if !a.IsOpenAIOAuth() {
		return ""
	}
	return strings.TrimSpace(a.GetExtraString("openai_session_id"))
}
