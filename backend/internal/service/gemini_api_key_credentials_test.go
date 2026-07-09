package service

import "testing"

func TestNormalizeGeminiAPIKeyCredentialsMarksCustomBaseURLAsRelay(t *testing.T) {
	in := map[string]any{
		"api_key":  "sk-test",
		"base_url": " https://iacc.cc/ ",
		"tier_id":  GeminiTierAIStudioFree,
	}

	out := NormalizeGeminiAPIKeyCredentials(PlatformGemini, AccountTypeAPIKey, in)

	if got := out["base_url"]; got != "https://iacc.cc" {
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

func TestNormalizeGeminiAPIKeyCredentialsAcceptsOpenAIClientAliases(t *testing.T) {
	out := NormalizeGeminiAPIKeyCredentials(PlatformGemini, AccountTypeOAuth, map[string]any{
		"apiKey":  "sk-test",
		"baseURL": " https://iacc.cc/v1/models?unused=1 ",
	})

	if got := out["api_key"]; got != "sk-test" {
		t.Fatalf("api_key = %q, want alias value", got)
	}
	if got := out["base_url"]; got != "https://iacc.cc/v1" {
		t.Fatalf("base_url = %q, want endpoint stripped to versioned base URL", got)
	}
	if got := out["tier_id"]; got != GeminiUpstreamCompatibleRelay {
		t.Fatalf("tier_id = %q, want %q", got, GeminiUpstreamCompatibleRelay)
	}
	if got := out["upstream_type"]; got != GeminiUpstreamCompatibleRelay {
		t.Fatalf("upstream_type = %q, want %q", got, GeminiUpstreamCompatibleRelay)
	}
}

func TestNormalizeGeminiAPIKeyAccountTypeAcceptsOpenAIClientAliases(t *testing.T) {
	got := NormalizeGeminiAPIKeyAccountType(PlatformGemini, AccountTypeOAuth, map[string]any{
		"apiKey":  "sk-test",
		"baseURL": "https://iacc.cc",
	})
	if got != AccountTypeAPIKey {
		t.Fatalf("NormalizeGeminiAPIKeyAccountType() = %q, want %q", got, AccountTypeAPIKey)
	}
}

func TestNormalizeGeminiAPIKeyAccountTypeTreatsAPIKeyPayloadAsAPIKey(t *testing.T) {
	tests := []struct {
		name        string
		platform    string
		accountType string
		credentials map[string]any
		want        string
	}{
		{
			name:        "gemini oauth with api key",
			platform:    PlatformGemini,
			accountType: AccountTypeOAuth,
			credentials: map[string]any{"api_key": "sk-test", "base_url": "https://iacc.cc"},
			want:        AccountTypeAPIKey,
		},
		{
			name:        "gemini missing type with api key",
			platform:    PlatformGemini,
			accountType: "",
			credentials: map[string]any{"api_key": "sk-test"},
			want:        AccountTypeAPIKey,
		},
		{
			name:        "gemini oauth without api key",
			platform:    PlatformGemini,
			accountType: AccountTypeOAuth,
			credentials: map[string]any{"refresh_token": "rt"},
			want:        AccountTypeOAuth,
		},
		{
			name:        "openai api key is untouched",
			platform:    PlatformOpenAI,
			accountType: AccountTypeOAuth,
			credentials: map[string]any{"api_key": "sk-test"},
			want:        AccountTypeOAuth,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NormalizeGeminiAPIKeyAccountType(tt.platform, tt.accountType, tt.credentials); got != tt.want {
				t.Fatalf("NormalizeGeminiAPIKeyAccountType() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNormalizeGeminiAPIKeyCredentialsMarksMislabelledOAuthAPIKeyRelay(t *testing.T) {
	out := NormalizeGeminiAPIKeyCredentials(PlatformGemini, AccountTypeOAuth, map[string]any{
		"api_key":  "sk-test",
		"base_url": "https://iacc.cc",
	})

	if got := out["tier_id"]; got != GeminiUpstreamCompatibleRelay {
		t.Fatalf("tier_id = %q, want %q", got, GeminiUpstreamCompatibleRelay)
	}
	if got := out["upstream_type"]; got != GeminiUpstreamCompatibleRelay {
		t.Fatalf("upstream_type = %q, want %q", got, GeminiUpstreamCompatibleRelay)
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
