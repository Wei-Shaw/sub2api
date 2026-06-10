package service

import "testing"

func TestNormalizeGeminiAPIKeyCredentialsMarksCustomBaseURLAsRelay(t *testing.T) {
	in := map[string]any{
		"api_key":  "sk-test",
		"base_url": " https://iacc.cc/ ",
		"tier_id":  GeminiTierAIStudioFree,
	}

	out := NormalizeGeminiAPIKeyCredentials(PlatformGemini, AccountTypeAPIKey, in)

	if got := out["base_url"]; got != "https://iacc.cc/" {
		t.Fatalf("base_url = %q, want trimmed iacc URL", got)
	}
	if got := out["tier_id"]; got != GeminiUpstreamCompatibleRelay {
		t.Fatalf("tier_id = %q, want %q", got, GeminiUpstreamCompatibleRelay)
	}
	if got := out["upstream_type"]; got != GeminiUpstreamCompatibleRelay {
		t.Fatalf("upstream_type = %q, want %q", got, GeminiUpstreamCompatibleRelay)
	}
	if _, exists := in["upstream_type"]; exists {
		t.Fatalf("NormalizeGeminiAPIKeyCredentials mutated input map")
	}
}

func TestNormalizeGeminiAPIKeyCredentialsKeepsOfficialBaseURLNative(t *testing.T) {
	out := NormalizeGeminiAPIKeyCredentials(PlatformGemini, AccountTypeAPIKey, map[string]any{
		"api_key":       "AIza-test",
		"base_url":      "https://generativelanguage.googleapis.com/v1beta/openai",
		"tier_id":       GeminiUpstreamCompatibleRelay,
		"upstream_type": GeminiUpstreamCompatibleRelay,
	})

	if _, exists := out["upstream_type"]; exists {
		t.Fatalf("upstream_type should be removed for official Gemini base URL")
	}
	if got := out["tier_id"]; got != GeminiTierAIStudioFree {
		t.Fatalf("tier_id = %q, want %q", got, GeminiTierAIStudioFree)
	}
}

func TestNormalizeGeminiAPIKeyCredentialsSkipsOtherAccountKinds(t *testing.T) {
	in := map[string]any{
		"api_key":  "sk-test",
		"base_url": "https://iacc.cc",
	}

	out := NormalizeGeminiAPIKeyCredentials(PlatformOpenAI, AccountTypeAPIKey, in)

	if got := out["base_url"]; got != "https://iacc.cc" {
		t.Fatalf("base_url = %q, want original value", got)
	}
	if _, exists := out["upstream_type"]; exists {
		t.Fatalf("upstream_type should not be added for non-Gemini accounts")
	}
}
