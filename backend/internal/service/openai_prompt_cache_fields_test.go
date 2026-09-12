package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeOpenAIPromptCacheFields(t *testing.T) {
	tests := []struct {
		name, model, platform, accountType string
		token, retention, options, key     bool
		wantRetention, wantOptions         bool
	}{
		{"legacy token", "gpt-5.5-2026-08-01", PlatformOpenAI, AccountTypeAPIKey, true, true, true, true, true, false},
		{"legacy default deny", "gpt-5.5", PlatformOpenAI, AccountTypeAPIKey, false, true, true, true, false, false},
		{"gpt 5.6", "gpt-5.6-2026-08-01", PlatformOpenAI, AccountTypeAPIKey, true, true, true, true, false, true},
		{"gpt 5.7", "gpt-5.7", PlatformOpenAI, AccountTypeAPIKey, true, true, true, true, false, true},
		{"gpt 5.6 oauth", "gpt-5.6", PlatformOpenAI, AccountTypeOAuth, true, true, true, true, false, false},
		{"gpt 5.6 grok", "gpt-5.6", PlatformGrok, AccountTypeAPIKey, true, true, true, true, false, false},
		{"gpt 5.6 third party keeps options", "gpt-5.6", PlatformOpenAI, AccountTypeAPIKey, false, true, true, true, false, true},
		{"oauth", "gpt-5.5", PlatformOpenAI, AccountTypeOAuth, true, true, true, true, false, false},
		{"grok", "gpt-5.5", PlatformGrok, AccountTypeAPIKey, true, true, true, true, false, false},
		{"unknown", "gpt-5.0-custom", PlatformOpenAI, AccountTypeAPIKey, true, true, true, true, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			credentials := map[string]any{}
			if tt.token {
				credentials[openAIEndpointCapabilitiesCredentialKey] = []string{"prompt_cache_retention"}
			}
			body := map[string]any{"model": tt.model, "prompt_cache_retention": "24h", "prompt_cache_options": map[string]any{"enabled": true}, "prompt_cache_key": "keep"}
			changed := normalizeOpenAIPromptCacheFields(body, &Account{Platform: tt.platform, Type: tt.accountType, Credentials: credentials}, tt.model)
			require.Equal(t, tt.wantRetention, body["prompt_cache_retention"] != nil)
			require.Equal(t, tt.wantOptions, body["prompt_cache_options"] != nil)
			require.Equal(t, "keep", body["prompt_cache_key"])
			require.Equal(t, tt.retention != tt.wantRetention || tt.options != tt.wantOptions, changed)
		})
	}
	t.Run("gpt 5.6 nil account", func(t *testing.T) {
		body := map[string]any{"model": "gpt-5.6", "prompt_cache_retention": "24h", "prompt_cache_options": map[string]any{"enabled": true}, "prompt_cache_key": "keep"}
		normalizeOpenAIPromptCacheFields(body, nil, "gpt-5.6")
		require.NotContains(t, body, "prompt_cache_retention")
		require.NotContains(t, body, "prompt_cache_options")
		require.Equal(t, "keep", body["prompt_cache_key"])
	})
	for _, model := range []string{"gpt-5.5-2-01", "gpt-5.5--2026-08-01", "gpt-5.5-2026-8-01"} {
		t.Run("invalid suffix "+model, func(t *testing.T) {
			a := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{openAIEndpointCapabilitiesCredentialKey: []string{"prompt_cache_retention"}}}
			body := map[string]any{"model": model, "prompt_cache_retention": "24h", "prompt_cache_options": map[string]any{"enabled": true}, "prompt_cache_key": "keep"}
			normalizeOpenAIPromptCacheFields(body, a, model)
			require.NotContains(t, body, "prompt_cache_retention")
			require.NotContains(t, body, "prompt_cache_options")
			require.Equal(t, "keep", body["prompt_cache_key"])
		})
	}
	for _, model := range []string{"gpt-5.6garbage", "gpt-5.7invalid"} {
		t.Run("invalid new model "+model, func(t *testing.T) {
			a := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
			body := map[string]any{"model": model, "prompt_cache_retention": "24h", "prompt_cache_options": map[string]any{"enabled": true}}
			normalizeOpenAIPromptCacheFields(body, a, model)
			require.NotContains(t, body, "prompt_cache_retention")
			require.NotContains(t, body, "prompt_cache_options")
		})
	}
	t.Run("session nested fields use captured mapped model when model omitted", func(t *testing.T) {
		a := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{openAIEndpointCapabilitiesCredentialKey: []string{"prompt_cache_retention"}}}
		body := []byte(`{"type":"session.update","session":{"prompt_cache_retention":"24h","prompt_cache_options":{"enabled":true},"prompt_cache_key":"keep"}}`)
		normalized, changed, err := normalizeOpenAIPromptCacheFieldsRawWithSessionModel(body, a, "", "gpt-5.5")
		require.NoError(t, err)
		require.True(t, changed)
		require.Contains(t, string(normalized), "prompt_cache_retention")
		require.NotContains(t, string(normalized), "prompt_cache_options")
		require.Contains(t, string(normalized), "prompt_cache_key")
	})
}
