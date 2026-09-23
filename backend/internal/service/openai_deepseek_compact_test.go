package service

import (
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestBuildDeepSeekCompactChatBody(t *testing.T) {
	body := []byte(`{"model":"gpt-5.6-sol","stream":true,"input":[
		{"type":"message","role":"user","content":"discuss a bot"},
		{"type":"compaction_trigger"}
	]}`)

	out, err := buildDeepSeekCompactChatBody(body)
	require.NoError(t, err)

	// trigger dropped
	require.False(t, gjson.GetBytes(out, "input.#(type==compaction_trigger)").Exists(), "trigger must be removed")
	// summary instruction appended as trailing user message
	input := gjson.GetBytes(out, "input").Array()
	require.NotEmpty(t, input)
	last := input[len(input)-1]
	require.Equal(t, "message", last.Get("type").String())
	require.Equal(t, "user", last.Get("role").String())
	require.Contains(t, last.Get("content.0.text").String(), "summary of the conversation")
	// forced non-streaming
	require.False(t, gjson.GetBytes(out, "stream").Bool())
}

func TestCompactSummaryTextFromResponses(t *testing.T) {
	output := []apicompat.ResponsesOutput{
		{Type: "reasoning", Summary: []apicompat.ResponsesSummary{{Type: "summary_text", Text: "thinking"}}},
		{Type: "message", Content: []apicompat.ResponsesContentPart{{Type: "output_text", Text: "the summary"}}},
	}
	require.Equal(t, "the summary", compactSummaryTextFromResponses(output))
	require.Equal(t, "", compactSummaryTextFromResponses([]apicompat.ResponsesOutput{{Type: "message", Content: nil}}))
}

func TestBuildDeepSeekCompactResponse(t *testing.T) {
	resp := &apicompat.ResponsesResponse{ID: "resp_1", Object: "response", Status: "in_progress", Output: []apicompat.ResponsesOutput{
		{Type: "reasoning"},
		{Type: "message", Content: []apicompat.ResponsesContentPart{{Type: "output_text", Text: "sum"}}},
	}}
	out := buildDeepSeekCompactResponse(resp, "sum")
	require.Equal(t, "completed", out.Status)
	require.Len(t, out.Output, 1)
	require.Equal(t, "compaction", out.Output[0].Type)
	require.Equal(t, "completed", out.Output[0].Status)
	require.Len(t, out.Output[0].Summary, 1)
	require.Equal(t, "sum", out.Output[0].Summary[0].Text)
	require.NotEmpty(t, out.Output[0].ID)

	raw, err := json.Marshal(out)
	require.NoError(t, err)
	require.NotEmpty(t, gjson.GetBytes(raw, "output.0.encrypted_content").String(), "deepseek compact item must carry encrypted_content")
	require.Equal(t, "compaction", gjson.GetBytes(raw, "output.0.type").String())
}
