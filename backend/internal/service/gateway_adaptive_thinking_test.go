package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestNormalizeAdaptiveThinkingForMappedModel(t *testing.T) {
	for _, model := range []string{"claude-fable-5-1", "claude-opus-5-5", "claude-opus-4-7", "claude-sonnet-5", "claude-sonnet-5-5"} {
		t.Run(model, func(t *testing.T) {
			req := &apicompat.AnthropicRequest{Thinking: &apicompat.AnthropicThinking{Type: "enabled", BudgetTokens: 10000}}
			NormalizeAdaptiveThinkingForMappedModel(req, model)
			require.Equal(t, "adaptive", req.Thinking.Type)
			require.Zero(t, req.Thinking.BudgetTokens)
		})
	}

	t.Run("legacy model keeps enabled", func(t *testing.T) {
		req := &apicompat.AnthropicRequest{Thinking: &apicompat.AnthropicThinking{Type: "enabled", BudgetTokens: 10000}}
		NormalizeAdaptiveThinkingForMappedModel(req, "claude-sonnet-4-6")
		require.Equal(t, "enabled", req.Thinking.Type)
		require.Equal(t, 10000, req.Thinking.BudgetTokens)
	})

	t.Run("adaptive and nil thinking are untouched", func(t *testing.T) {
		adaptive := &apicompat.AnthropicRequest{Thinking: &apicompat.AnthropicThinking{Type: "adaptive"}}
		NormalizeAdaptiveThinkingForMappedModel(adaptive, "claude-fable-5-1")
		require.Equal(t, "adaptive", adaptive.Thinking.Type)

		none := &apicompat.AnthropicRequest{}
		NormalizeAdaptiveThinkingForMappedModel(none, "claude-fable-5-1")
		require.Nil(t, none.Thinking)
	})
}

func TestNormalizeAdaptiveOnlyThinking(t *testing.T) {
	t.Run("adaptive-only model rewrites enabled and drops budget", func(t *testing.T) {
		body := []byte(`{"model":"claude-sonnet-5","thinking":{"type":"enabled","budget_tokens":8192},"messages":[]}`)
		out, applied := NormalizeAdaptiveOnlyThinking(body, "claude-sonnet-5")
		require.True(t, applied)
		require.Equal(t, "adaptive", gjson.GetBytes(out, "thinking.type").String())
		require.False(t, gjson.GetBytes(out, "thinking.budget_tokens").Exists())
		require.Equal(t, "claude-sonnet-5", gjson.GetBytes(out, "model").String(), "model and other fields must be preserved")
	})

	t.Run("mapped fable model rewrites enabled", func(t *testing.T) {
		body := []byte(`{"model":"claude-opus-4-6","thinking":{"type":"enabled","budget_tokens":2048},"messages":[]}`)
		out, applied := NormalizeAdaptiveOnlyThinking(body, "claude-fable-5-1")
		require.True(t, applied)
		require.Equal(t, "adaptive", gjson.GetBytes(out, "thinking.type").String())
		require.False(t, gjson.GetBytes(out, "thinking.budget_tokens").Exists())
	})

	t.Run("legacy model stays byte-identical", func(t *testing.T) {
		body := []byte(`{"model":"claude-opus-4-6","thinking":{"type":"enabled","budget_tokens":2048},"messages":[]}`)
		out, applied := NormalizeAdaptiveOnlyThinking(body, "claude-opus-4-6")
		require.False(t, applied)
		require.Equal(t, body, out)
	})

	t.Run("adaptive and missing thinking are no-ops", func(t *testing.T) {
		for _, body := range [][]byte{
			[]byte(`{"thinking":{"type":"adaptive"},"messages":[]}`),
			[]byte(`{"thinking":{"type":"disabled"},"messages":[]}`),
			[]byte(`{"messages":[]}`),
		} {
			out, applied := NormalizeAdaptiveOnlyThinking(body, "claude-sonnet-5")
			require.False(t, applied)
			require.Equal(t, body, out)
		}
	})
}
