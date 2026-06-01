//go:build unit

package admin

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateAPIKeyChatCompletionsCredentialsAuthHeader(t *testing.T) {
	baseCreds := func() map[string]any {
		return map[string]any{
			"chat_completions_url": "https://compat-upstream.example/v1/chat/completions",
			"api_key":              "sk-test",
		}
	}

	for _, value := range []string{"", "Authorization", "authorization", "api-key", "x-api-key"} {
		t.Run("allows_"+value, func(t *testing.T) {
			creds := baseCreds()
			creds["auth_header"] = value
			require.NoError(t, validateAPIKeyChatCompletionsCredentials(creds))
		})
	}

	for _, value := range []string{"X-Custom-Key", "api-key\nInjected: yes", "Authorization: Bearer"} {
		t.Run("rejects_"+value, func(t *testing.T) {
			creds := baseCreds()
			creds["auth_header"] = value
			require.ErrorContains(t, validateAPIKeyChatCompletionsCredentials(creds), "auth_header must be one of")
		})
	}
}
