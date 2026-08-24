package kiro

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDefaultModels_MatchesKiroReferenceModels(t *testing.T) {
	ids := make([]string, 0, len(DefaultModels))
	for _, model := range DefaultModels {
		ids = append(ids, model.ID)
	}

	require.Equal(t, []string{
		"gpt-5.6-sol",
		"gpt-5.6-terra",
		"gpt-5.6-luna",
		"claude-opus-4-8",
		"claude-opus-4-8-thinking",
		"claude-opus-4-7",
		"claude-opus-4-7-thinking",
		"claude-opus-4-6",
		"claude-opus-4-6-thinking",
		"claude-opus-5",
		"claude-opus-5-thinking",
		"claude-sonnet-5",
		"claude-sonnet-5-thinking",
		"claude-sonnet-4-6",
		"claude-sonnet-4-6-thinking",
		"claude-opus-4-5-20251101",
		"claude-opus-4-5-20251101-thinking",
		"claude-sonnet-4-5-20250929",
		"claude-sonnet-4-5-20250929-thinking",
		"claude-haiku-4-5-20251001",
		"claude-haiku-4-5-20251001-thinking",
	}, ids)

	require.Contains(t, ids, "gpt-5.6-sol")
	require.Contains(t, ids, "gpt-5.6-terra")
	require.Contains(t, ids, "gpt-5.6-luna")
	require.Contains(t, ids, "claude-sonnet-4-6")
	require.Contains(t, ids, "claude-opus-4-7")
	require.Contains(t, ids, "claude-opus-4-8")
	require.Contains(t, ids, "claude-opus-5")
	require.Contains(t, ids, "claude-opus-5-thinking")
	require.Contains(t, ids, "claude-haiku-4-5-20251001-thinking")
	require.NotContains(t, ids, "gpt-5.6")
	require.NotContains(t, ids, "auto")
	require.NotContains(t, ids, "claude-sonnet-4")
	require.NotContains(t, ids, "gpt-4o")
	require.NotContains(t, ids, "deepseek-3-2")
	require.NotContains(t, ids, "minimax-m2-1")
	require.NotContains(t, ids, "qwen3-coder-next")
	require.NotContains(t, ids, "claude-sonnet-4-6-chat")
	displayNames := make(map[string]string, len(DefaultModels))
	for _, model := range DefaultModels {
		displayNames[model.ID] = model.DisplayName
	}
	require.Equal(t, "GPT-5.6 Sol", displayNames["gpt-5.6-sol"])
	require.Equal(t, "GPT-5.6 Terra", displayNames["gpt-5.6-terra"])
	require.Equal(t, "GPT-5.6 Luna", displayNames["gpt-5.6-luna"])
}
