//go:build unit

package service

import (
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestDevinRejectsContentLostByCompatibilityConversion(t *testing.T) {
	cases := []struct{ protocol, body string }{
		{APIProtocolResponses, `{"input":[{"role":"user","content":[{"type":"input_file","file_id":"file-1"}]}]}`},
		{APIProtocolResponses, `{"store":true,"input":"hi"}`},
		{APIProtocolResponses, `{"context_management":[{"type":"compaction"}],"input":"hi"}`},
		{APIProtocolResponses, `{"input":[{"role":"assistant","content":[{"type":"input_image","image_url":"data:image/png;base64,eA=="}]}]}`},
		{APIProtocolResponses, `{"input":[{"type":"function_call","call_id":"call","name":"f","arguments":"{}"}]}`},
		{APIProtocolResponses, `{"input":[{"type":"function_call_output","call_id":"orphan","output":"x"}]}`},
		{APIProtocolAnthropic, `{"messages":[{"role":"other","content":"hi"}]}`},
		{APIProtocolAnthropic, `{"messages":[{"role":"user","content":[{"type":"image","source":{"type":"url","url":"https://example.com/a.png"}}]}]}`},
		{APIProtocolAnthropic, `{"messages":[{"role":"user","content":[{"type":"tool_result","tool_use_id":"call","content":[{"type":"document","text":"secret"}]}]}]}`},
		{APIProtocolAnthropic, `{"system":[{"type":"image","source":{"type":"base64","data":"eA==","media_type":"image/png"}}]}`},
		{APIProtocolAnthropic, `{"messages":[{"role":"assistant","content":[{"type":"tool_use","id":"call","name":"f","input":{}}]}]}`},
	}
	for _, tc := range cases {
		require.Error(t, validateDevinInbound([]byte(tc.body), tc.protocol), tc.body)
	}
}

func TestDevinAnthropicHistoryPreservesReasoningAndToolErrors(t *testing.T) {
	original := []byte(`{"model":"swe-1-6","messages":[
		{"role":"user","content":"hello"},
		{"role":"assistant","content":[{"type":"thinking","thinking":"reasoning without tool"},{"type":"text","text":"first answer"}]},
		{"role":"user","content":"check"},
		{"role":"assistant","content":[{"type":"thinking","thinking":"tool reasoning"},{"type":"tool_use","id":"call","name":"f","input":{}}]},
		{"role":"user","content":[{"type":"tool_result","tool_use_id":"call","is_error":true,"content":"failed"}]}
	]}`)
	require.NoError(t, validateDevinInbound(original, APIProtocolAnthropic))
	var request apicompat.AnthropicRequest
	require.NoError(t, json.Unmarshal(original, &request))
	chat, err := apicompat.AnthropicToChatCompletionsRequest(&request)
	require.NoError(t, err)
	body, err := json.Marshal(chat)
	require.NoError(t, err)
	body, err = restoreDevinAnthropicHistory(body, original)
	require.NoError(t, err)
	require.Equal(t, "reasoning without tool", gjson.GetBytes(body, "messages.1.reasoning_content").String())
	require.Equal(t, "tool reasoning", gjson.GetBytes(body, "messages.3.reasoning_content").String())
	require.True(t, gjson.GetBytes(body, "messages.4.is_error").Bool())
	require.Equal(t, "call", gjson.GetBytes(body, "messages.4.tool_call_id").String())
}
