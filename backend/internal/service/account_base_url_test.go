//go:build unit

package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"github.com/stretchr/testify/require"
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

func TestGetGrokBaseURLUsesSubscriptionProxyForOAuth(t *testing.T) {
	tests := []struct {
		name     string
		account  Account
		expected string
	}{
		{
			name: "oauth without base_url uses CLI subscription proxy",
			account: Account{
				Type:        AccountTypeOAuth,
				Platform:    PlatformGrok,
				Credentials: map[string]any{},
			},
			expected: xai.DefaultCLIBaseURL,
		},
		{
			name: "oauth legacy API default is migrated at runtime to CLI subscription proxy",
			account: Account{
				Type:     AccountTypeOAuth,
				Platform: PlatformGrok,
				Credentials: map[string]any{
					"base_url": xai.DefaultBaseURL,
				},
			},
			expected: xai.DefaultCLIBaseURL,
		},
		{
			name: "oauth legacy API default with trailing slash is migrated at runtime",
			account: Account{
				Type:     AccountTypeOAuth,
				Platform: PlatformGrok,
				Credentials: map[string]any{
					"base_url": xai.DefaultBaseURL + "/",
				},
			},
			expected: xai.DefaultCLIBaseURL,
		},
		{
			name: "oauth legacy API root is migrated at runtime",
			account: Account{
				Type:     AccountTypeOAuth,
				Platform: PlatformGrok,
				Credentials: map[string]any{
					"base_url": "https://api.x.ai",
				},
			},
			expected: xai.DefaultCLIBaseURL,
		},
		{
			name: "oauth legacy API root with canonical HTTPS port is migrated at runtime",
			account: Account{
				Type:     AccountTypeOAuth,
				Platform: PlatformGrok,
				Credentials: map[string]any{
					"base_url": "HTTPS://API.X.AI:443/",
				},
			},
			expected: xai.DefaultCLIBaseURL,
		},
		{
			name: "oauth legacy API canonical port with leading zeroes is migrated at runtime",
			account: Account{
				Type:     AccountTypeOAuth,
				Platform: PlatformGrok,
				Credentials: map[string]any{
					"base_url": "https://api.x.ai:0443/v1",
				},
			},
			expected: xai.DefaultCLIBaseURL,
		},
		{
			name: "oauth legacy API encoded version path is migrated at runtime",
			account: Account{
				Type:     AccountTypeOAuth,
				Platform: PlatformGrok,
				Credentials: map[string]any{
					"base_url": "https://api.x.ai/%76%31",
				},
			},
			expected: xai.DefaultCLIBaseURL,
		},
		{
			name: "oauth legacy API encoded trailing slash is migrated at runtime",
			account: Account{
				Type:     AccountTypeOAuth,
				Platform: PlatformGrok,
				Credentials: map[string]any{
					"base_url": "https://api.x.ai/v1%2F",
				},
			},
			expected: xai.DefaultCLIBaseURL,
		},
		{
			name: "oauth non-default API port remains pinned to CLI proxy",
			account: Account{
				Type:     AccountTypeOAuth,
				Platform: PlatformGrok,
				Credentials: map[string]any{
					"base_url": "https://api.x.ai:8443/v1",
				},
			},
			expected: xai.DefaultCLIBaseURL,
		},
		{
			name: "oauth explicit custom base_url stays pinned to CLI proxy by default",
			account: Account{
				Type:     AccountTypeOAuth,
				Platform: PlatformGrok,
				Credentials: map[string]any{
					"base_url": "https://custom.example.com/v1",
				},
			},
			expected: xai.DefaultCLIBaseURL,
		},
		{
			name: "API key without base_url uses official credit-backed API",
			account: Account{
				Type:        AccountTypeAPIKey,
				Platform:    PlatformGrok,
				Credentials: map[string]any{},
			},
			expected: xai.DefaultBaseURL,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expected, tt.account.GetGrokBaseURL())
		})
	}
}

func TestGetGrokBaseURLPinsOAuthWhenUnsafeOverridesEnabled(t *testing.T) {
	t.Setenv(xai.EnvAllowUnsafeURLOverrides, "true")
	account := Account{
		Type:     AccountTypeOAuth,
		Platform: PlatformGrok,
		Credentials: map[string]any{
			"base_url": "https://custom.example.com/v1",
		},
	}

	require.Equal(t, xai.DefaultCLIBaseURL, account.GetGrokBaseURL())
}

func TestGetGrokMediaBaseURLPinsOAuthMediaToCLIProxy(t *testing.T) {
	tests := []struct {
		name     string
		account  Account
		expected string
	}{
		{
			name: "oauth without base_url uses CLI subscription proxy",
			account: Account{
				Type:        AccountTypeOAuth,
				Platform:    PlatformGrok,
				Credentials: map[string]any{},
			},
			expected: xai.DefaultCLIBaseURL,
		},
		{
			name: "oauth stored CLI proxy stays on CLI subscription proxy",
			account: Account{
				Type:     AccountTypeOAuth,
				Platform: PlatformGrok,
				Credentials: map[string]any{
					"base_url": xai.DefaultCLIBaseURL,
				},
			},
			expected: xai.DefaultCLIBaseURL,
		},
		{
			name: "oauth stored CLI proxy variant is canonicalized to CLI proxy",
			account: Account{
				Type:     AccountTypeOAuth,
				Platform: PlatformGrok,
				Credentials: map[string]any{
					"base_url": "HTTPS://CLI-CHAT-PROXY.GROK.COM:443/%76%31/",
				},
			},
			expected: xai.DefaultCLIBaseURL,
		},
		{
			name: "oauth legacy official API is pinned to CLI proxy",
			account: Account{
				Type:     AccountTypeOAuth,
				Platform: PlatformGrok,
				Credentials: map[string]any{
					"base_url": xai.DefaultBaseURL,
				},
			},
			expected: xai.DefaultCLIBaseURL,
		},
		{
			name: "oauth untrusted custom base_url is pinned to CLI proxy",
			account: Account{
				Type:     AccountTypeOAuth,
				Platform: PlatformGrok,
				Credentials: map[string]any{
					"base_url": "https://custom.example.com/v1",
				},
			},
			expected: xai.DefaultCLIBaseURL,
		},
		{
			name: "API key retains its configured media API",
			account: Account{
				Type:     AccountTypeAPIKey,
				Platform: PlatformGrok,
				Credentials: map[string]any{
					"base_url": "https://grok.example.com/v1",
				},
			},
			expected: "https://grok.example.com/v1",
		},
		{
			name: "non-Grok account has no Grok media base URL",
			account: Account{
				Type:        AccountTypeOAuth,
				Platform:    PlatformOpenAI,
				Credentials: map[string]any{},
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expected, tt.account.GetGrokMediaBaseURL())
		})
	}
}

func TestGetGrokMediaBaseURLPinsOAuthWhenUnsafeOverridesEnabled(t *testing.T) {
	t.Setenv(xai.EnvAllowUnsafeURLOverrides, "true")
	account := Account{
		Type:     AccountTypeOAuth,
		Platform: PlatformGrok,
		Credentials: map[string]any{
			"base_url": "https://custom.example.com/v1",
		},
	}

	require.Equal(t, xai.DefaultCLIBaseURL, account.GetGrokMediaBaseURL())
}
