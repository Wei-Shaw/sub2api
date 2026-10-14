package antigravity

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestApplyGeminiThinkingConfig(t *testing.T) {
	t.Run("Gemini 3 uses thinking level", func(t *testing.T) {
		body := []byte(`{"contents":[],"generationConfig":{"temperature":0.3,"thinkingConfig":{"thinkingBudget":4096}}}`)

		got, err := ApplyGeminiThinkingConfig(body, "gemini-3.8-flash-high", "high")

		require.NoError(t, err)
		require.Equal(t, "HIGH", gjson.GetBytes(got, "generationConfig.thinkingConfig.thinkingLevel").String())
		require.True(t, gjson.GetBytes(got, "generationConfig.thinkingConfig.includeThoughts").Bool())
		require.False(t, gjson.GetBytes(got, "generationConfig.thinkingConfig.thinkingBudget").Exists())
		require.Equal(t, 0.3, gjson.GetBytes(got, "generationConfig.temperature").Float())
	})

	t.Run("Gemini 2.5 uses thinking budget", func(t *testing.T) {
		body := []byte(`{"contents":[]}`)

		got, err := ApplyGeminiThinkingConfig(body, "gemini-2.5-flash", "medium")

		require.NoError(t, err)
		require.Equal(t, int64(gemini25ThinkingBudgetMedium), gjson.GetBytes(got, "generationConfig.thinkingConfig.thinkingBudget").Int())
		require.True(t, gjson.GetBytes(got, "generationConfig.thinkingConfig.includeThoughts").Bool())
		require.False(t, gjson.GetBytes(got, "generationConfig.thinkingConfig.thinkingLevel").Exists())
	})

	t.Run("unsupported model is unchanged", func(t *testing.T) {
		body := []byte(`{"contents":[]}`)

		got, err := ApplyGeminiThinkingConfig(body, "gemini-3.1-flash-image", "high")

		require.NoError(t, err)
		require.JSONEq(t, string(body), string(got))
	})

	t.Run("unknown level is unchanged", func(t *testing.T) {
		body := []byte(`{"contents":[]}`)

		got, err := ApplyGeminiThinkingConfig(body, "gemini-3.8-flash", "unknown")

		require.NoError(t, err)
		require.JSONEq(t, string(body), string(got))
	})
}
