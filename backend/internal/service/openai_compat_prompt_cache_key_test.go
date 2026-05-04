package service

import (
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/stretchr/testify/require"
)

func mustRawJSON(t *testing.T, s string) json.RawMessage {
	t.Helper()
	return json.RawMessage(s)
}

func TestShouldAutoInjectPromptCacheKeyForCompat(t *testing.T) {
	require.True(t, shouldAutoInjectPromptCacheKeyForCompat("gpt-5.4"))
	require.True(t, shouldAutoInjectPromptCacheKeyForCompat("gpt-5.3"))
	require.True(t, shouldAutoInjectPromptCacheKeyForCompat("gpt-5.3-codex"))
	require.True(t, shouldAutoInjectPromptCacheKeyForCompat("gpt-5.3-codex-spark"))
	require.False(t, shouldAutoInjectPromptCacheKeyForCompat("gpt-4o"))
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

func TestDeriveCompatPromptCacheKey_IncludesAugmentedBuiltinTools(t *testing.T) {
	base := &apicompat.ChatCompletionsRequest{
		Model: "gpt-5.4",
		Messages: []apicompat.ChatMessage{
			{Role: "user", Content: mustRawJSON(t, `"hello"`)},
		},
	}
	withBuiltin := *base
	withBuiltin.BuiltinTools = true
	withExplicitTool := *base
	withExplicitTool.Tools = []apicompat.ChatTool{{Type: "web_search"}}

	baseKey := deriveCompatPromptCacheKey(base, "gpt-5.4")
	builtinKey := deriveCompatPromptCacheKey(&withBuiltin, "gpt-5.4")
	explicitKey := deriveCompatPromptCacheKey(&withExplicitTool, "gpt-5.4")

	require.NotEqual(t, baseKey, builtinKey)
	require.Equal(t, explicitKey, builtinKey)
}

func TestDeriveCompatPromptCacheKey_BuiltinToolsAffectsSupportedTools(t *testing.T) {
	base := &apicompat.ChatCompletionsRequest{
		Model: "gpt-5.4",
		Messages: []apicompat.ChatMessage{
			{Role: "user", Content: mustRawJSON(t, `"hello"`)},
		},
	}
	withUnsupported := *base
	withUnsupported.BuiltinTools = []any{"code_interpreter"}
	withWebSearch := *base
	withWebSearch.BuiltinTools = []any{"web_search", "image_generation"}
	withImageGeneration := *base
	withImageGeneration.BuiltinTools = map[string]any{"image_generation": map[string]any{"enabled": true, "model": "gpt-image-2", "output_format": "png"}}

	baseKey := deriveCompatPromptCacheKey(base, "gpt-5.4")
	unsupportedKey := deriveCompatPromptCacheKey(&withUnsupported, "gpt-5.4")
	webSearchKey := deriveCompatPromptCacheKey(&withWebSearch, "gpt-5.4")
	imageGenerationKey := deriveCompatPromptCacheKey(&withImageGeneration, "gpt-5.4")

	require.Equal(t, baseKey, unsupportedKey)
	require.NotEqual(t, baseKey, webSearchKey)
	require.NotEqual(t, baseKey, imageGenerationKey)
	require.NotEqual(t, webSearchKey, imageGenerationKey)
}
