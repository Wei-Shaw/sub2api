package service

import (
	"net/url"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

var deepSeekDefaultModels = [...]string{
	"deepseek-v4-flash",
	"deepseek-v4-pro",
}

// DeepSeekDefaultModelIDs returns the built-in model catalog used before an
// account has supplied an explicit model list.
func DeepSeekDefaultModelIDs() []string {
	models := make([]string, len(deepSeekDefaultModels))
	copy(models, deepSeekDefaultModels[:])
	return models
}

// normalizeAccountPlatform canonicalizes account input before provider-specific
// validation. Account rows never use the synthetic composite platform.
func normalizeAccountPlatform(platform string) (string, error) {
	platform = strings.ToLower(strings.TrimSpace(platform))
	switch platform {
	case PlatformAnthropic,
		PlatformOpenAI,
		PlatformGemini,
		PlatformAntigravity,
		PlatformGrok,
		PlatformDeepSeek,
		PlatformKimi,
		PlatformZhipu,
		PlatformMiniMax,
		PlatformOpenCodeGo:
		return platform, nil
	default:
		return "", infraerrors.BadRequest("ACCOUNT_PLATFORM_INVALID", "account platform is not supported")
	}
}

// normalizeAccountCredentialsForPlatform enforces provider-specific account
// identity invariants at the service boundary. Callers receive a new map so a
// failed request never partially mutates the supplied credentials.
func normalizeAccountCredentialsForPlatform(platform, accountType string, credentials map[string]any) (map[string]any, error) {
	if platform != PlatformDeepSeek {
		return credentials, nil
	}
	if accountType != AccountTypeAPIKey {
		return nil, infraerrors.BadRequest(
			"DEEPSEEK_ACCOUNT_TYPE_INVALID",
			"DeepSeek accounts only support apikey credentials",
		)
	}

	apiKeyValue, ok := credentials["api_key"]
	if !ok || apiKeyValue == nil {
		return nil, infraerrors.BadRequest("DEEPSEEK_API_KEY_REQUIRED", "DeepSeek api_key is required")
	}
	apiKey, ok := apiKeyValue.(string)
	if !ok {
		return nil, infraerrors.BadRequest("DEEPSEEK_API_KEY_INVALID", "DeepSeek api_key must be a string")
	}
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return nil, infraerrors.BadRequest("DEEPSEEK_API_KEY_REQUIRED", "DeepSeek api_key is required")
	}
	for i := 0; i < len(apiKey); i++ {
		if apiKey[i] < 0x21 || apiKey[i] > 0x7e {
			return nil, infraerrors.BadRequest(
				"DEEPSEEK_API_KEY_INVALID",
				"DeepSeek api_key must contain only printable ASCII characters without whitespace",
			)
		}
	}

	baseURL := DefaultDeepSeekBaseURL
	if rawBaseURL, exists := credentials["base_url"]; exists && rawBaseURL != nil {
		value, valueOK := rawBaseURL.(string)
		if !valueOK {
			return nil, infraerrors.BadRequest("DEEPSEEK_BASE_URL_INVALID", "DeepSeek base_url must be a string")
		}
		if strings.TrimSpace(value) != "" {
			baseURL = value
		}
	}
	normalizedBaseURL, err := normalizeDeepSeekBaseURL(baseURL)
	if err != nil {
		return nil, err
	}

	normalized := make(map[string]any, len(credentials)+1)
	for key, value := range credentials {
		normalized[key] = value
	}
	normalized["api_key"] = apiKey
	normalized["base_url"] = normalizedBaseURL
	return normalized, nil
}

func normalizeDeepSeekBaseURL(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed == nil || parsed.Host == "" || parsed.Opaque != "" {
		return "", infraerrors.BadRequest("DEEPSEEK_BASE_URL_INVALID", "DeepSeek base_url must be an absolute HTTP(S) URL")
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", infraerrors.BadRequest("DEEPSEEK_BASE_URL_INVALID", "DeepSeek base_url must use HTTP or HTTPS")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", infraerrors.BadRequest(
			"DEEPSEEK_BASE_URL_INVALID",
			"DeepSeek base_url must not contain user info, query parameters, or a fragment",
		)
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	parsed.RawPath = ""
	if strings.EqualFold(parsed.Hostname(), "api.deepseek.com") && parsed.Path == "/v1" {
		// The OpenAI SDK commonly documents this alias. Accounts store one
		// protocol-neutral root, so canonicalize it before deriving the native
		// Chat, Responses, and Anthropic endpoints.
		parsed.Path = ""
	}
	lowerPath := strings.ToLower(parsed.Path)
	for _, suffix := range []string{"/v1", "/models", "/chat/completions", "/responses", "/anthropic/v1/messages"} {
		if lowerPath == suffix || strings.HasSuffix(lowerPath, suffix) {
			return "", infraerrors.BadRequest(
				"DEEPSEEK_BASE_URL_INVALID",
				"DeepSeek base_url must be the shared API root without a version or endpoint suffix",
			)
		}
	}
	return strings.TrimRight(parsed.String(), "/"), nil
}
