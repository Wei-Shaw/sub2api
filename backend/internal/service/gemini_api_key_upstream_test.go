//go:build unit

package service

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildGeminiAPIKeyUpstreamURL(t *testing.T) {
	validate := func(raw string) (string, error) { return raw, nil }

	tests := []struct {
		name     string
		account  *Account
		model    string
		action   string
		stream   bool
		expected string
	}{
		{
			name: "legacy api key keeps ai studio endpoint",
			account: &Account{
				Type:        AccountTypeAPIKey,
				Platform:    PlatformGemini,
				Credentials: map[string]any{},
			},
			model:    "gemini-2.5-flash",
			action:   "generateContent",
			expected: "https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-flash:generateContent",
		},
		{
			name: "legacy ai studio custom model path remains unchanged",
			account: &Account{
				Type:        AccountTypeAPIKey,
				Platform:    PlatformGemini,
				Credentials: map[string]any{},
			},
			model:    "custom/models/gemini-relay",
			action:   "generateContent",
			expected: "https://generativelanguage.googleapis.com/v1beta/models/custom/models/gemini-relay:generateContent",
		},
		{
			name: "vertex api key uses stable express endpoint",
			account: &Account{
				Type:        AccountTypeAPIKey,
				Platform:    PlatformGemini,
				Credentials: map[string]any{"api_mode": "vertex"},
			},
			model:    "gemini-2.5-flash",
			action:   "generateContent",
			expected: "https://aiplatform.googleapis.com/v1/publishers/google/models/gemini-2.5-flash:generateContent",
		},
		{
			name: "vertex streaming uses sse query",
			account: &Account{
				Type:        AccountTypeAPIKey,
				Platform:    PlatformGemini,
				Credentials: map[string]any{"api_mode": "vertex"},
			},
			model:    "models/gemini-2.5-pro",
			action:   "streamGenerateContent",
			stream:   true,
			expected: "https://aiplatform.googleapis.com/v1/publishers/google/models/gemini-2.5-pro:streamGenerateContent?alt=sse",
		},
		{
			name: "vertex accepts full publisher model resource",
			account: &Account{
				Type:        AccountTypeAPIKey,
				Platform:    PlatformGemini,
				Credentials: map[string]any{"api_mode": "vertex"},
			},
			model:    "publishers/google/models/gemini-2.5-flash",
			action:   "countTokens",
			expected: "https://aiplatform.googleapis.com/v1/publishers/google/models/gemini-2.5-flash:countTokens",
		},
		{
			name: "custom vertex relay base url is preserved",
			account: &Account{
				Type:     AccountTypeAPIKey,
				Platform: PlatformGemini,
				Credentials: map[string]any{
					"api_mode": "vertex",
					"base_url": "https://relay.example.com/root/",
				},
			},
			model:    "gemini-2.5-flash",
			action:   "generateContent",
			expected: "https://relay.example.com/root/v1/publishers/google/models/gemini-2.5-flash:generateContent",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildGeminiAPIKeyUpstreamURL(tt.account, validate, tt.model, tt.action, tt.stream)
			require.NoError(t, err)
			require.Equal(t, tt.expected, got)
		})
	}
}

func TestBuildGeminiAPIKeyUpstreamURLRejectsInvalidInput(t *testing.T) {
	validate := func(raw string) (string, error) { return raw, nil }
	account := &Account{
		Type:        AccountTypeAPIKey,
		Platform:    PlatformGemini,
		Credentials: map[string]any{"api_mode": "vertex"},
	}

	_, err := buildGeminiAPIKeyUpstreamURL(nil, validate, "gemini-2.5-flash", "generateContent", false)
	require.ErrorContains(t, err, "account is nil")

	_, err = buildGeminiAPIKeyUpstreamURL(account, validate, "", "generateContent", false)
	require.ErrorContains(t, err, "missing model")

	_, err = buildGeminiAPIKeyUpstreamURL(account, validate, "gemini-2.5-flash", "deleteModel", false)
	require.ErrorContains(t, err, "unsupported gemini action")
}

func TestBuildGeminiVertexModelsFallback(t *testing.T) {
	result, handled, err := buildGeminiVertexModelsFallback("/v1beta/models")
	require.NoError(t, err)
	require.True(t, handled)
	require.Equal(t, http.StatusOK, result.StatusCode)
	require.Contains(t, string(result.Body), `"models"`)

	result, handled, err = buildGeminiVertexModelsFallback("/v1beta/models/gemini-2.5-flash")
	require.NoError(t, err)
	require.True(t, handled)
	require.Equal(t, http.StatusOK, result.StatusCode)
	require.Contains(t, string(result.Body), `"models/gemini-2.5-flash"`)

	result, handled, err = buildGeminiVertexModelsFallback("/v1beta/files")
	require.NoError(t, err)
	require.False(t, handled)
	require.Nil(t, result)
}
