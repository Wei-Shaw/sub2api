package service

import (
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/stretchr/testify/require"
)

func TestStripChatHistoryReasoning(t *testing.T) {
	body := []byte(`{"model":"gemini-3.8-flash","messages":[
		{"role":"user","content":"hi"},
		{"role":"assistant","content":"Hello","reasoning_content":"PRIOR_REASONING"},
		{"role":"assistant","content":"Again","reasoning":"ALIAS_REASONING"},
		{"role":"assistant","content":[{"type":"thinking","thinking":"PART_THINKING"},{"type":"reasoning","text":"PART_REASONING"},{"type":"text","text":"Third"}]},
		{"role":"user","content":"next"}
	]}`)
	var req apicompat.ChatCompletionsRequest
	require.NoError(t, json.Unmarshal(body, &req))

	stripChatHistoryReasoning(&req)

	require.Empty(t, req.Messages[1].ReasoningContent)
	require.Empty(t, req.Messages[2].Reasoning)
	require.JSONEq(t, `"Hello"`, string(req.Messages[1].Content), "string content must survive untouched")

	parts, err := json.Marshal(req.Messages[3].Content)
	require.NoError(t, err)
	require.NotContains(t, string(parts), "PART_THINKING")
	require.NotContains(t, string(parts), "PART_REASONING")
	require.Contains(t, string(parts), "Third")
}

func TestStripChatHistoryReasoningNilRequest(t *testing.T) {
	require.NotPanics(t, func() { stripChatHistoryReasoning(nil) })
}
