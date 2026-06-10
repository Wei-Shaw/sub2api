package service

import "strings"

// NormalizeGeminiAPIKeyCredentials keeps all Gemini API-key entry points aligned.
// Custom Gemini base URLs are compatible relays and must use the OpenAI-compatible path.
func NormalizeGeminiAPIKeyCredentials(platform, accountType string, credentials map[string]any) map[string]any {
	if credentials == nil {
		return nil
	}

	out := make(map[string]any, len(credentials)+2)
	for key, value := range credentials {
		out[key] = value
	}

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
