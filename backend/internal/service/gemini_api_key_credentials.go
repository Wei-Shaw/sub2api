package service

import (
	"net/url"
	"strings"
)

// NormalizeOpenAICompatibleCredentialAliases accepts common OpenAI-client
// credential field names and stores the canonical keys used internally.
func NormalizeOpenAICompatibleCredentialAliases(platform, accountType string, credentials map[string]any) map[string]any {
	if credentials == nil {
		return nil
	}

	out := make(map[string]any, len(credentials)+2)
	for key, value := range credentials {
		out[key] = value
	}

	switch platform {
	case PlatformOpenAI, PlatformGemini:
	default:
		return out
	}

	if strings.TrimSpace(credentialString(out, "api_key")) == "" {
		if apiKey := firstCredentialString(out, []string{
			"apiKey",
			"api-key",
			"apikey",
			"key",
			"openai_api_key",
			"openaiApiKey",
			"OPENAI_API_KEY",
			"gemini_api_key",
			"geminiApiKey",
			"GEMINI_API_KEY",
		}); apiKey != "" {
			out["api_key"] = apiKey
		}
	}

	if strings.TrimSpace(credentialString(out, "base_url")) == "" {
		if baseURL := firstCredentialString(out, []string{
			"baseURL",
			"baseUrl",
			"api_base_url",
			"apiBaseUrl",
			"apiBaseURL",
			"api_base",
			"apiBase",
			"openai_base_url",
			"openaiBaseUrl",
			"OPENAI_BASE_URL",
			"gemini_base_url",
			"geminiBaseUrl",
			"GEMINI_BASE_URL",
			"endpoint",
			"api_url",
			"apiUrl",
			"url",
		}); baseURL != "" {
			out["base_url"] = baseURL
		}
	}

	if baseURL := strings.TrimSpace(credentialString(out, "base_url")); baseURL != "" {
		out["base_url"] = normalizeOpenAICompatibleBaseURL(baseURL)
	}

	return out
}

// NormalizeGeminiAPIKeyAccountType keeps imported/API-created Gemini API-key
// accounts usable even when older tools accidentally label them as OAuth.
func NormalizeGeminiAPIKeyAccountType(platform, accountType string, credentials map[string]any) string {
	accountType = strings.TrimSpace(accountType)
	if platform == PlatformGemini {
		credentials = NormalizeOpenAICompatibleCredentialAliases(platform, accountType, credentials)
	}
	if platform != PlatformGemini || !hasCredentialString(credentials, "api_key") {
		return accountType
	}
	if accountType == "" ||
		accountType == AccountTypeAPIKey ||
		accountType == AccountTypeOAuth ||
		accountType == AccountTypeSetupToken ||
		accountType == AccountTypeUpstream {
		return AccountTypeAPIKey
	}
	return accountType
}

// NormalizeGeminiAPIKeyCredentials keeps all Gemini API-key entry points aligned.
// Custom Gemini base URLs are compatible relays and must use the OpenAI-compatible path.
func NormalizeGeminiAPIKeyCredentials(platform, accountType string, credentials map[string]any) map[string]any {
	if credentials == nil {
		return nil
	}

	out := NormalizeOpenAICompatibleCredentialAliases(platform, accountType, credentials)

	accountType = NormalizeGeminiAPIKeyAccountType(platform, accountType, out)
	if platform != PlatformGemini || accountType != AccountTypeAPIKey {
		return out
	}

	account := Account{Credentials: out}
	baseURL := strings.TrimSpace(account.GetCredential("base_url"))
	if baseURL != "" {
		out["base_url"] = baseURL
	}

	if baseURL != "" && !isOfficialGeminiBaseURL(baseURL) {
		out["tier_id"] = GeminiUpstreamCompatibleRelay
		out["upstream_type"] = GeminiUpstreamCompatibleRelay
		return out
	}

	upstreamType := strings.TrimSpace(account.GetCredential("upstream_type"))
	if strings.EqualFold(upstreamType, GeminiUpstreamCompatibleRelay) {
		delete(out, "upstream_type")
	}

	tierID := strings.TrimSpace(account.GetCredential("tier_id"))
	if tierID == "" || strings.EqualFold(tierID, GeminiUpstreamCompatibleRelay) {
		out["tier_id"] = GeminiTierAIStudioFree
	}

	return out
}

// NormalizeGeminiAPIKeyAccount returns a runtime-safe copy for legacy Gemini
// API-key accounts that were imported or saved with an OAuth-like type.
func NormalizeGeminiAPIKeyAccount(account *Account) *Account {
	if account == nil {
		return nil
	}
	if account.Platform != PlatformGemini || !hasCredentialString(account.Credentials, "api_key") {
		accountCopy := *account
		accountCopy.Credentials = NormalizeOpenAICompatibleCredentialAliases(account.Platform, account.Type, account.Credentials)
		if accountCopy.Platform != PlatformGemini || !hasCredentialString(accountCopy.Credentials, "api_key") {
			return &accountCopy
		}
		account = &accountCopy
	}

	accountCopy := *account
	accountCopy.Type = NormalizeGeminiAPIKeyAccountType(account.Platform, account.Type, account.Credentials)
	accountCopy.Credentials = NormalizeGeminiAPIKeyCredentials(account.Platform, accountCopy.Type, account.Credentials)
	return &accountCopy
}

func NormalizeGeminiAPIKeyAccountValue(account Account) Account {
	normalized := NormalizeGeminiAPIKeyAccount(&account)
	if normalized == nil {
		return account
	}
	return *normalized
}

func NormalizeGeminiAPIKeyAccounts(accounts []Account) []Account {
	if len(accounts) == 0 {
		return accounts
	}
	out := make([]Account, len(accounts))
	for i, account := range accounts {
		out[i] = NormalizeGeminiAPIKeyAccountValue(account)
	}
	return out
}

func NormalizeGeminiAPIKeyAccountPointers(accounts []*Account) []*Account {
	if len(accounts) == 0 {
		return accounts
	}
	out := make([]*Account, len(accounts))
	for i, account := range accounts {
		out[i] = NormalizeGeminiAPIKeyAccount(account)
	}
	return out
}

func hasCredentialString(credentials map[string]any, key string) bool {
	if credentials == nil {
		return false
	}
	return strings.TrimSpace(credentialString(credentials, key)) != ""
}

func firstCredentialString(credentials map[string]any, keys []string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(credentialString(credentials, key)); value != "" {
			return value
		}
	}
	return ""
}

func credentialString(credentials map[string]any, key string) string {
	if credentials == nil {
		return ""
	}
	value, ok := credentials[key]
	if !ok {
		return ""
	}
	switch v := value.(type) {
	case string:
		return v
	case []byte:
		return string(v)
	default:
		return ""
	}
}

func normalizeOpenAICompatibleBaseURL(raw string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(raw), "/")
	if trimmed == "" {
		return ""
	}

	if parsed, err := url.Parse(trimmed); err == nil && parsed.Scheme != "" && parsed.Host != "" {
		parsed.Path = stripOpenAICompatibleEndpointPath(parsed.Path)
		parsed.RawQuery = ""
		parsed.Fragment = ""
		return strings.TrimRight(parsed.String(), "/")
	}

	return strings.TrimRight(stripOpenAICompatibleEndpointPath(trimmed), "/")
}

func stripOpenAICompatibleEndpointPath(pathValue string) string {
	pathValue = strings.TrimRight(strings.TrimSpace(pathValue), "/")
	if pathValue == "" {
		return pathValue
	}
	prefix := ""
	if strings.HasPrefix(pathValue, "/") {
		prefix = "/"
	}
	segments := strings.Split(strings.Trim(pathValue, "/"), "/")
	if len(segments) == 0 {
		return pathValue
	}

	for _, endpoint := range [][]string{
		{"chat", "completions"},
		{"images", "generations"},
		{"images", "edits"},
		{"images", "variations"},
		{"audio", "speech"},
		{"audio", "transcriptions"},
		{"audio", "translations"},
		{"models"},
		{"responses"},
		{"embeddings"},
	} {
		if len(segments) < len(endpoint) {
			continue
		}
		start := len(segments) - len(endpoint)
		matched := true
		for i, want := range endpoint {
			if !strings.EqualFold(segments[start+i], want) {
				matched = false
				break
			}
		}
		if matched {
			segments = segments[:start]
			if len(segments) == 0 {
				return ""
			}
			return prefix + strings.Join(segments, "/")
		}
	}

	return pathValue
}
