//go:build unit

package apicompat

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestDeepSeekRequestFieldDecoding(t *testing.T) {
	for _, field := range []string{"title", "safety_check"} {
		for _, value := range []string{`"x"`, `"high"`, "true", "false", "17", "[]", "null", `{"unknown":9007199254740993}`} {
			t.Run(field+"/"+value, func(t *testing.T) {
				wire := []byte(fmt.Sprintf(`{"model":"test","messages":[{"role":"user","content":"hi"}],"input":"hi","%s":%s}`, field, value))
				var chat ChatCompletionsRequest
				var responses ResponsesRequest
				var messages AnthropicRequest
				for _, req := range []any{&chat, &responses, &messages} {
					require.NoError(t, json.Unmarshal(wire, req))
				}
				asResponses, err := ChatCompletionsToResponses(&chat)
				require.NoError(t, err)
				asChat, err := ResponsesToChatCompletionsRequest(&responses)
				require.NoError(t, err)
				asMessages, err := ResponsesToAnthropicRequest(&responses)
				require.NoError(t, err)
				fromMessages, err := AnthropicToChatCompletionsRequest(&messages)
				require.NoError(t, err)
				for _, converted := range []any{asResponses, asChat, asMessages, fromMessages} {
					out, err := json.Marshal(converted)
					require.NoError(t, err)
					v := gjson.GetBytes(out, field)
					if value[0] == '{' {
						require.Equal(t, value, v.Raw)
					} else {
						require.False(t, v.Exists())
					}
				}
			})
		}
	}
}

func TestDeepSeekChatWebHistory(t *testing.T) {
	const input = `[
		{"role":"user","content":"read these"},
		{"type":"reasoning","content":[{"type":"reasoning_text","text":"look up"}]},
		{"type":"web_search_call","id":"s","kind":"search","action":{"type":"search","queries":["q"],"sources":[{"url":"u"}]},"status":"completed"},
		{"type":"reasoning","content":[{"type":"reasoning_text","text":"read both"}]},
		{"type":"web_search_call","id":"a","kind":"read_link","action":{"type":"open_page","url":"a"},"page":{"title":"A"},"signature":"opaque+/=","status":"completed"},
		{"type":"web_search_call","id":"b","kind":"read_link","action":{"type":"open_page","url":"b"},"error":{"code":"fetch_failed"},"status":"failed"},
		{"type":"reasoning","content":[{"type":"reasoning_text","text":"answer"}]},
		{"type":"message","role":"assistant","content":[{"type":"output_text","text":"done"}]},
		{"role":"user","content":"next"},
		{"type":"web_search_call","id":"c","kind":"read_link","action":{"type":"open_page","url":"c"},"signature":"signed","status":"completed"},
		{"role":"user","content":"after refusal"}
	]`
	req := &ResponsesRequest{Input: json.RawMessage(input)}
	out, err := ResponsesToChatCompletionsRequestWithOptions(req, &ResponsesToChatOptions{PreserveWebToolHistory: true})
	require.NoError(t, err)
	raw, err := json.Marshal(out.Messages)
	require.NoError(t, err)
	require.Len(t, out.Messages, 5)
	require.Equal(t, "answer", out.Messages[1].ReasoningContent)
	require.JSONEq(t, `[
		{"index":0,"reasoning_content":"look up","calls":[{"index":0,"id":"s","type":"deepseek_web_search","kind":"search","action":{"type":"search","queries":["q"]},"sources":[{"url":"u"}],"status":"completed"}]},
		{"index":1,"reasoning_content":"read both","calls":[
			{"index":0,"id":"a","type":"deepseek_read_url","kind":"read_link","action":{"type":"read_link","url":"a"},"page":{"title":"A"},"signature":"opaque+/=","status":"completed"},
			{"index":1,"id":"b","type":"deepseek_read_url","kind":"read_link","action":{"type":"read_link","url":"b"},"error":{"code":"fetch_failed"},"status":"failed"}]}
	]`, gjson.GetBytes(raw, "1.server_tool_calls").Raw)
	require.Equal(t, "assistant", out.Messages[3].Role)
	require.Equal(t, "signed", gjson.GetBytes(raw, "3.server_tool_calls.0.calls.0.signature").String())
	other, err := ResponsesToChatCompletionsRequest(req)
	require.NoError(t, err)
	raw, err = json.Marshal(other)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "server_tool_calls")
	require.Len(t, other.Messages, 4)
}
