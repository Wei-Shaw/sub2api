package service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/stretchr/testify/require"
)

func mustRawJSON(t *testing.T, s string) json.RawMessage {
	t.Helper()
	return json.RawMessage(s)
}

func TestShouldAutoInjectPromptCacheKeyForCompat(t *testing.T) {
	require.True(t, shouldAutoInjectPromptCacheKeyForCompat("gpt-5.5"))
	require.True(t, shouldAutoInjectPromptCacheKeyForCompat("gpt-5.5-pro"))
	require.True(t, shouldAutoInjectPromptCacheKeyForCompat("gpt-5.4"))
	require.True(t, shouldAutoInjectPromptCacheKeyForCompat("gpt-5.4-mini"))
	require.True(t, shouldAutoInjectPromptCacheKeyForCompat("gpt-5.2"))
	require.True(t, shouldAutoInjectPromptCacheKeyForCompat("gpt-5.3"))
	require.True(t, shouldAutoInjectPromptCacheKeyForCompat("gpt-5.3-codex"))
	require.True(t, shouldAutoInjectPromptCacheKeyForCompat("gpt-5.3-codex-spark"))
	require.False(t, shouldAutoInjectPromptCacheKeyForCompat("gpt-4o"))
}

func TestResolveOpenAIResponsesPromptCacheMode(t *testing.T) {
	account := func(platform, accountType, mode string) *Account {
		result := &Account{Platform: platform, Type: accountType}
		if mode != "" {
			result.Extra = map[string]any{OpenAIPromptCacheModeExtraKey: mode}
		}
		return result
	}
	tests := []struct {
		name                string
		account             *Account
		wantMode            string
		wantMappedMarkers   bool
		wantSentBreakpoints bool
		wantExplicitOptions bool
	}{
		{
			name:     "nil account",
			wantMode: OpenAIPromptCacheModeOff,
		},
		{
			name:     "missing mode",
			account:  account(PlatformOpenAI, AccountTypeAPIKey, ""),
			wantMode: OpenAIPromptCacheModeOff,
		},
		{
			name:     "explicit off",
			account:  account(PlatformOpenAI, AccountTypeAPIKey, OpenAIPromptCacheModeOff),
			wantMode: OpenAIPromptCacheModeOff,
		},
		{
			name:     "invalid mode",
			account:  account(PlatformOpenAI, AccountTypeAPIKey, "automatic"),
			wantMode: OpenAIPromptCacheModeOff,
		},
		{
			name:     "mode is exact",
			account:  account(PlatformOpenAI, AccountTypeAPIKey, " implicit_breakpoints "),
			wantMode: OpenAIPromptCacheModeOff,
		},
		{
			name: "non string mode",
			account: &Account{
				Platform: PlatformOpenAI,
				Type:     AccountTypeAPIKey,
				Extra:    map[string]any{OpenAIPromptCacheModeExtraKey: true},
			},
			wantMode: OpenAIPromptCacheModeOff,
		},
		{
			name:              "stable key only",
			account:           account(PlatformOpenAI, AccountTypeAPIKey, OpenAIPromptCacheModeStableKeyOnly),
			wantMode:          OpenAIPromptCacheModeStableKeyOnly,
			wantMappedMarkers: true,
		},
		{
			name:                "implicit breakpoints",
			account:             account(PlatformOpenAI, AccountTypeAPIKey, OpenAIPromptCacheModeImplicitBreakpoints),
			wantMode:            OpenAIPromptCacheModeImplicitBreakpoints,
			wantMappedMarkers:   true,
			wantSentBreakpoints: true,
		},
		{
			name:                "explicit only",
			account:             account(PlatformOpenAI, AccountTypeAPIKey, OpenAIPromptCacheModeExplicitOnly),
			wantMode:            OpenAIPromptCacheModeExplicitOnly,
			wantMappedMarkers:   true,
			wantSentBreakpoints: true,
			wantExplicitOptions: true,
		},
		{
			name:     "OpenAI OAuth",
			account:  account(PlatformOpenAI, AccountTypeOAuth, OpenAIPromptCacheModeExplicitOnly),
			wantMode: OpenAIPromptCacheModeOff,
		},
		{
			name:     "Grok API key",
			account:  account(PlatformGrok, AccountTypeAPIKey, OpenAIPromptCacheModeExplicitOnly),
			wantMode: OpenAIPromptCacheModeOff,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.wantMode, resolveOpenAIResponsesPromptCacheMode(tt.account))
			require.Equal(t, tt.wantMappedMarkers, mapsAnthropicPromptCacheBreakpoints(tt.account))
			require.Equal(t, tt.wantSentBreakpoints, sendsOpenAIResponsesPromptCacheBreakpoints(tt.account))
			require.Equal(t, tt.wantExplicitOptions, sendsOpenAIResponsesPromptCacheOptions(tt.account))
		})
	}
}

func TestDeriveCompatPromptCacheKey_StableAcrossLaterTurns(t *testing.T) {
	base := &apicompat.ChatCompletionsRequest{
		Model: "gpt-5.4",
		Messages: []apicompat.ChatMessage{
			{Role: "system", Content: mustRawJSON(t, `"You are helpful."`)},
			{Role: "user", Content: mustRawJSON(t, `"Hello"`)},
		},
	}
	extended := &apicompat.ChatCompletionsRequest{
		Model: "gpt-5.4",
		Messages: []apicompat.ChatMessage{
			{Role: "system", Content: mustRawJSON(t, `"You are helpful."`)},
			{Role: "user", Content: mustRawJSON(t, `"Hello"`)},
			{Role: "assistant", Content: mustRawJSON(t, `"Hi there!"`)},
			{Role: "user", Content: mustRawJSON(t, `"How are you?"`)},
		},
	}

	k1 := deriveCompatPromptCacheKey(base, "gpt-5.4")
	k2 := deriveCompatPromptCacheKey(extended, "gpt-5.4")
	require.Equal(t, k1, k2, "cache key should be stable across later turns")
	require.NotEmpty(t, k1)
}

func TestDeriveCompatPromptCacheKey_DiffersAcrossSessions(t *testing.T) {
	req1 := &apicompat.ChatCompletionsRequest{
		Model: "gpt-5.4",
		Messages: []apicompat.ChatMessage{
			{Role: "user", Content: mustRawJSON(t, `"Question A"`)},
		},
	}
	req2 := &apicompat.ChatCompletionsRequest{
		Model: "gpt-5.4",
		Messages: []apicompat.ChatMessage{
			{Role: "user", Content: mustRawJSON(t, `"Question B"`)},
		},
	}

	k1 := deriveCompatPromptCacheKey(req1, "gpt-5.4")
	k2 := deriveCompatPromptCacheKey(req2, "gpt-5.4")
	require.NotEqual(t, k1, k2, "different first user messages should yield different keys")
}

func TestDeriveCompatPromptCacheKey_UsesResolvedSparkFamily(t *testing.T) {
	req := &apicompat.ChatCompletionsRequest{
		Model: "gpt-5.3-codex-spark",
		Messages: []apicompat.ChatMessage{
			{Role: "user", Content: mustRawJSON(t, `"Question A"`)},
		},
	}

	k1 := deriveCompatPromptCacheKey(req, "gpt-5.3-codex-spark")
	k2 := deriveCompatPromptCacheKey(req, " openai/gpt-5.3-codex-spark ")
	require.NotEmpty(t, k1)
	require.Equal(t, k1, k2, "resolved spark family should derive a stable compat cache key")
}

func TestDeriveAnthropicCompatPromptCacheKey_StableAcrossLaterTurns(t *testing.T) {
	base := &apicompat.AnthropicRequest{
		Model:  "claude-sonnet-4-5",
		System: mustRawJSON(t, `"You are helpful."`),
		Messages: []apicompat.AnthropicMessage{
			{Role: "user", Content: mustRawJSON(t, `"Open repo"`)},
		},
	}
	extended := &apicompat.AnthropicRequest{
		Model:  "claude-sonnet-4-5",
		System: mustRawJSON(t, `"You are helpful."`),
		Messages: []apicompat.AnthropicMessage{
			{Role: "user", Content: mustRawJSON(t, `"Open repo"`)},
			{Role: "assistant", Content: mustRawJSON(t, `"Opened."`)},
			{Role: "user", Content: mustRawJSON(t, `"Run tests"`)},
		},
	}

	k1 := deriveAnthropicCompatPromptCacheKey(base, "gpt-5.3-codex")
	k2 := deriveAnthropicCompatPromptCacheKey(extended, "gpt-5.3-codex")
	require.NotEmpty(t, k1)
	require.Equal(t, k1, k2, "cache key should stay stable as later Claude Code turns append history")
}

func TestDeriveAnthropicCompatPromptCacheKey_UsesCacheControlAnchors(t *testing.T) {
	base := &apicompat.AnthropicRequest{
		Model: "claude-sonnet-4-5",
		System: mustRawJSON(t, `[
			{"type":"text","text":"project instructions","cache_control":{"type":"ephemeral"}}
		]`),
		Messages: []apicompat.AnthropicMessage{
			{Role: "user", Content: mustRawJSON(t, `[
				{"type":"text","text":"repo anchor","cache_control":{"type":"ephemeral"}}
			]`)},
		},
	}
	extended := &apicompat.AnthropicRequest{
		Model:  base.Model,
		System: base.System,
		Messages: []apicompat.AnthropicMessage{
			base.Messages[0],
			{Role: "assistant", Content: mustRawJSON(t, `[{"type":"text","text":"Opened."}]`)},
			{Role: "user", Content: mustRawJSON(t, `[{"type":"text","text":"Run tests"}]`)},
		},
	}

	k1 := deriveAnthropicCompatPromptCacheKey(base, "gpt-5.4")
	k2 := deriveAnthropicCompatPromptCacheKey(extended, "gpt-5.4")
	require.NotEmpty(t, k1)
	require.Equal(t, k1, k2)
	require.True(t, strings.HasPrefix(k1, "anthropic-cache-"))
	require.False(t, strings.HasPrefix(k1, compatPromptCacheKeyPrefix))
}
