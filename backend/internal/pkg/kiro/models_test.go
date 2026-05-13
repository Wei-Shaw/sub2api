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
		"claude-opus-4-6",
		"claude-opus-4-6-thinking",
		"claude-sonnet-4-6",
		"claude-sonnet-4-6-thinking",
		"claude-opus-4-5-20251101",
		"claude-opus-4-5-20251101-thinking",
		"claude-sonnet-4-5-20250929",
		"claude-sonnet-4-5-20250929-thinking",
		"claude-haiku-4-5-20251001",
		"claude-haiku-4-5-20251001-thinking",
	}, ids)

	require.Contains(t, ids, "claude-sonnet-4-6")
	require.Contains(t, ids, "claude-haiku-4-5-20251001-thinking")
	require.NotContains(t, ids, "auto")
	require.NotContains(t, ids, "claude-sonnet-4")
	require.NotContains(t, ids, "gpt-4o")
	require.NotContains(t, ids, "deepseek-3-2")
	require.NotContains(t, ids, "minimax-m2-1")
	require.NotContains(t, ids, "qwen3-coder-next")
	require.NotContains(t, ids, "claude-opus-4-7")
	require.NotContains(t, ids, "claude-sonnet-4-6-chat")
	for _, id := range ids {
		require.NotContains(t, id, "kiro-")
		require.NotContains(t, id, "-agentic")
		require.NotContains(t, id, "-chat")
	}
}
