package service

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/stretchr/testify/require"
)

func TestApplyAnthropicCompatFullReplayGuard_TrimsOldMessages(t *testing.T) {
	t.Parallel()

	req := &apicompat.AnthropicRequest{Messages: make([]apicompat.AnthropicMessage, 0, openAICompatAnthropicReplayMaxTailMessages+3)}
	for i := 0; i < openAICompatAnthropicReplayMaxTailMessages+3; i++ {
		req.Messages = append(req.Messages, apicompat.AnthropicMessage{
			Role:    "user",
			Content: json.RawMessage(fmt.Sprintf(`"message-%02d"`, i)),
		})
	}

	trimmed := applyAnthropicCompatFullReplayGuard(req)

	require.True(t, trimmed)
	require.Len(t, req.Messages, openAICompatAnthropicReplayMaxTailMessages)
	require.JSONEq(t, `"message-03"`, string(req.Messages[0].Content))
	require.JSONEq(t, `"message-14"`, string(req.Messages[len(req.Messages)-1].Content))
}

func TestApplyAnthropicCompatFullReplayGuard_KeepsToolBoundaryIntact(t *testing.T) {
	t.Parallel()

	req := &apicompat.AnthropicRequest{Messages: make([]apicompat.AnthropicMessage, 0, openAICompatAnthropicReplayMaxTailMessages+3)}
	for i := 0; i < openAICompatAnthropicReplayMaxTailMessages+3; i++ {
		role := "user"
		content := json.RawMessage(fmt.Sprintf(`"message-%02d"`, i))
		if i == 1 {
			role = "assistant"
			content = json.RawMessage(`[{"type":"tool_use","id":"toolu_keep","name":"Read","input":{"file_path":"main.go"}}]`)
		}
		if i == 3 {
			content = json.RawMessage(`[{"type":"tool_result","tool_use_id":"toolu_keep","content":"ok"}]`)
		}
		req.Messages = append(req.Messages, apicompat.AnthropicMessage{
			Role:    role,
			Content: content,
		})
	}

	trimmed := applyAnthropicCompatFullReplayGuard(req)

	require.True(t, trimmed)
	require.Len(t, req.Messages, openAICompatAnthropicReplayMaxTailMessages+2)
	require.Equal(t, "assistant", req.Messages[0].Role)
	require.Contains(t, string(req.Messages[0].Content), `"toolu_keep"`)
	require.Contains(t, string(req.Messages[2].Content), `"tool_result"`)
}
