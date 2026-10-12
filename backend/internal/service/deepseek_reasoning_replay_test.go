package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDeepSeekResponsesCleanupPreservesPlaintextReasoning(t *testing.T) {
	for _, model := range []string{"deepseek-v4-flash", "DeepSeek-V4-Pro", "deepseek-ai/deepseek-v4-flash"} {
		body := []byte(`{"model":"` + model + `","store":false,"input":[{"type":"reasoning","id":"rs_test","content":[{"type":"reasoning_text","text":"original plan"}]},{"type":"function_call","call_id":"call_one","name":"lookup","arguments":"{}"}]}`)
		out, changed, err := normalizeOpenAIAPIKeyStoreFalseReasoningReplay(body, false)
		require.NoError(t, err)
		require.False(t, changed)
		out, changed, err = normalizeOpenAIResponsesReasoningContentReplay(out)
		require.NoError(t, err)
		require.False(t, changed)
		require.Equal(t, body, out)
	}
	require.False(t, isDeepSeekReasoningReplayModel("gpt-5.6-sol"))
}
