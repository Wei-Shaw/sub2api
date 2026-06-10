//go:build unit

package service

import (
	"context"
	"testing"
)

func TestGetBaseURL(t *testing.T) {
	tests := []struct {
		name     string
		account  Account
		expected string
	}{
		{
			name: "non-apikey type returns empty",
			account: Account{
				Type:     AccountTypeOAuth,
				Platform: PlatformAnthropic,
			},
			expected: "",
		},
		{
			name: "apikey without base_url returns default anthropic",
			account: Account{
				Type:        AccountTypeAPIKey,
				Platform:    PlatformAnthropic,
				Credentials: map[string]any{},
			},
			expected: "https://api.anthropic.com",
		},
		{
			name: "apikey with custom base_url",
			account: Account{
				Type:        AccountTypeAPIKey,
				Platform:    PlatformAnthropic,
				Credentials: map[string]any{"base_url": "https://custom.example.com"},
			},
			expected: "https://custom.example.com",
		},
		{
			name: "antigravity apikey auto-appends /antigravity",
			account: Account{
				Type:        AccountTypeAPIKey,
				Platform:    PlatformAntigravity,
				Credentials: map[string]any{"base_url": "https://upstream.example.com"},
			},
			expected: "https://upstream.example.com/antigravity",
		},
		{
			name: "antigravity apikey trims trailing slash before appending",
			account: Account{
				Type:        AccountTypeAPIKey,
				Platform:    PlatformAntigravity,
				Credentials: map[string]any{"base_url": "https://upstream.example.com/"},
			},
			expected: "https://upstream.example.com/antigravity",
		},
		{
			name: "antigravity non-apikey returns empty",
			account: Account{
				Type:        AccountTypeOAuth,
				Platform:    PlatformAntigravity,
				Credentials: map[string]any{"base_url": "https://upstream.example.com"},
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.account.GetBaseURL()
			if result != tt.expected {
				t.Errorf("GetBaseURL() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestGetGeminiBaseURL(t *testing.T) {
	const defaultGeminiURL = "https://generativelanguage.googleapis.com"

	tests := []struct {
		name     string
		account  Account
		expected string
	}{
		{
			name: "apikey without base_url returns default",
			account: Account{
				Type:        AccountTypeAPIKey,
				Platform:    PlatformGemini,
				Credentials: map[string]any{},
			},
			expected: defaultGeminiURL,
		},
		{
			name: "apikey with custom base_url",
			account: Account{
				Type:        AccountTypeAPIKey,
				Platform:    PlatformGemini,
				Credentials: map[string]any{"base_url": "https://custom-gemini.example.com"},
			},
			expected: "https://custom-gemini.example.com",
		},
		{
			name: "antigravity apikey auto-appends /antigravity",
			account: Account{
				Type:        AccountTypeAPIKey,
				Platform:    PlatformAntigravity,
				Credentials: map[string]any{"base_url": "https://upstream.example.com"},
			},
			expected: "https://upstream.example.com/antigravity",
		},
		{
			name: "antigravity apikey trims trailing slash",
			account: Account{
				Type:        AccountTypeAPIKey,
				Platform:    PlatformAntigravity,
				Credentials: map[string]any{"base_url": "https://upstream.example.com/"},
			},
			expected: "https://upstream.example.com/antigravity",
		},
		{
			name: "antigravity oauth does NOT append /antigravity",
			account: Account{
				Type:        AccountTypeOAuth,
				Platform:    PlatformAntigravity,
				Credentials: map[string]any{"base_url": "https://upstream.example.com"},
			},
			expected: "https://upstream.example.com",
		},
		{
			name: "oauth without base_url returns default",
			account: Account{
				Type:        AccountTypeOAuth,
				Platform:    PlatformAntigravity,
				Credentials: map[string]any{},
			},
			expected: defaultGeminiURL,
		},
		{
			name: "nil credentials returns default",
			account: Account{
				Type:     AccountTypeAPIKey,
				Platform: PlatformGemini,
			},
			expected: defaultGeminiURL,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.account.GetGeminiBaseURL(defaultGeminiURL)
			if result != tt.expected {
				t.Errorf("GetGeminiBaseURL() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestIsGeminiCompatibleRelay(t *testing.T) {
	tests := []struct {
		name    string
		account Account
		want    bool
	}{
		{
			name: "official apikey without marker is not relay",
			account: Account{
				Type:        AccountTypeAPIKey,
				Platform:    PlatformGemini,
				Credentials: map[string]any{"base_url": "https://generativelanguage.googleapis.com/v1beta"},
			},
			want: false,
		},
		{
			name: "official openai compatible apikey without marker is not relay",
			account: Account{
				Type:        AccountTypeAPIKey,
				Platform:    PlatformGemini,
				Credentials: map[string]any{"base_url": "https://generativelanguage.googleapis.com/v1beta/openai"},
			},
			want: false,
		},
		{
			name: "explicit upstream type marker is relay",
			account: Account{
				Type:        AccountTypeAPIKey,
				Platform:    PlatformGemini,
				Credentials: map[string]any{"upstream_type": GeminiUpstreamCompatibleRelay},
			},
			want: true,
		},
		{
			name: "explicit tier marker is relay",
			account: Account{
				Type:        AccountTypeAPIKey,
				Platform:    PlatformGemini,
				Credentials: map[string]any{"tier_id": GeminiUpstreamCompatibleRelay},
			},
			want: true,
		},
		{
			name: "custom base url is relay",
			account: Account{
				Type:        AccountTypeAPIKey,
				Platform:    PlatformGemini,
				Credentials: map[string]any{"base_url": "https://relay.example.com"},
			},
			want: true,
		},
		{
			name: "gemini oauth with custom base url is not relay",
			account: Account{
				Type:        AccountTypeOAuth,
				Platform:    PlatformGemini,
				Credentials: map[string]any{"base_url": "https://relay.example.com"},
			},
			want: false,
		},
		{
			name: "other platform is not gemini relay",
			account: Account{
				Type:        AccountTypeAPIKey,
				Platform:    PlatformOpenAI,
				Credentials: map[string]any{"base_url": "https://relay.example.com"},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.account.IsGeminiCompatibleRelay(); got != tt.want {
				t.Fatalf("IsGeminiCompatibleRelay() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGeminiOpenAICompatibleUpstream(t *testing.T) {
	tests := []struct {
		name       string
		account    Account
		want       bool
		wantRelay  bool
		wantNative string
	}{
		{
			name: "official native",
			account: Account{
				Type:        AccountTypeAPIKey,
				Platform:    PlatformGemini,
				Credentials: map[string]any{"base_url": "https://generativelanguage.googleapis.com/v1beta"},
			},
			want:       false,
			wantRelay:  false,
			wantNative: "https://generativelanguage.googleapis.com/v1beta",
		},
		{
			name: "official openai compatible",
			account: Account{
				Type:        AccountTypeAPIKey,
				Platform:    PlatformGemini,
				Credentials: map[string]any{"base_url": "https://generativelanguage.googleapis.com/v1beta/openai"},
			},
			want:       true,
			wantRelay:  false,
			wantNative: "https://generativelanguage.googleapis.com/v1beta",
		},
		{
			name: "third party relay",
			account: Account{
				Type:        AccountTypeAPIKey,
				Platform:    PlatformGemini,
				Credentials: map[string]any{"base_url": "https://relay.example.com"},
			},
			want:       true,
			wantRelay:  true,
			wantNative: "https://relay.example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.account.IsGeminiOpenAICompatibleUpstream(); got != tt.want {
				t.Fatalf("IsGeminiOpenAICompatibleUpstream() = %v, want %v", got, tt.want)
			}
			if got := tt.account.IsGeminiCompatibleRelay(); got != tt.wantRelay {
				t.Fatalf("IsGeminiCompatibleRelay() = %v, want %v", got, tt.wantRelay)
			}
			baseURL := tt.account.GetGeminiBaseURL("https://generativelanguage.googleapis.com")
			if got := geminiNativeBaseURLFromOpenAICompatible(baseURL); got != tt.wantNative {
				t.Fatalf("geminiNativeBaseURLFromOpenAICompatible() = %q, want %q", got, tt.wantNative)
			}
		})
	}
}

func TestGeminiQuotaSkipsCompatibleRelay(t *testing.T) {
	svc := NewGeminiQuotaService(nil, nil)

	official := &Account{
		Type:        AccountTypeAPIKey,
		Platform:    PlatformGemini,
		Credentials: map[string]any{"base_url": "https://generativelanguage.googleapis.com"},
	}
	if _, ok := svc.QuotaForAccount(context.Background(), official); !ok {
		t.Fatal("official Gemini API key should use local AI Studio quota policy")
	}

	relay := &Account{
		Type:        AccountTypeAPIKey,
		Platform:    PlatformGemini,
		Credentials: map[string]any{"base_url": "https://relay.example.com"},
	}
	if _, ok := svc.QuotaForAccount(context.Background(), relay); ok {
		t.Fatal("Gemini compatible relay should not use local Google quota policy")
	}
}
