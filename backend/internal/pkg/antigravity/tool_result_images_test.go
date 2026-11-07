package antigravity

import (
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestResponsesToolResultImagesStayWithTheirCalls(t *testing.T) {
	for _, model := range []string{"gemini-3.8-flash", "gemini-4-argon"} {
		t.Run(model, func(t *testing.T) {
			raw := []byte(`{"model":"gemini-3.8-flash-low","input":[
 {"type":"function_call","call_id":"call_A","name":"images","arguments":"{}"},
 {"type":"function_call","call_id":"call_B","name":"images","arguments":"{}"},
 {"type":"function_call_output","call_id":"call_A","output":[{"type":"input_text","text":"first pair"},{"type":"input_image","image_url":"data:image/png;base64,AQID"},{"type":"input_image","image_url":"data:image/webp;base64,BAUG"}]},
 {"type":"function_call_output","call_id":"call_B","output":[{"type":"input_image","image_url":"data:image/jpeg;base64,BwgJ"}]}
 ]}`)
			var req apicompat.ResponsesRequest
			require.NoError(t, json.Unmarshal(raw, &req))
			anthropic, err := apicompat.ResponsesToAnthropicRequest(&req)
			require.NoError(t, err)
			body, err := json.Marshal(anthropic)
			require.NoError(t, err)
			var claude ClaudeRequest
			require.NoError(t, json.Unmarshal(body, &claude))
			wire, err := TransformClaudeToGeminiWithOptions(&claude, "test-project", model, TransformOptions{})
			require.NoError(t, err)
			responses := map[string]gjson.Result{}
			for _, content := range gjson.GetBytes(wire, "request.contents").Array() {
				for _, part := range content.Get("parts").Array() {
					if fr := part.Get("functionResponse"); fr.Exists() {
						responses[fr.Get("id").String()] = fr
					}
					require.False(t, part.Get("inlineData").Exists(), "tool images must remain nested in their response")
				}
			}
			require.Len(t, responses, 2)
			a := responses["call_A"]
			b := responses["call_B"]
			require.Equal(t, "images", a.Get("name").String())
			require.Equal(t, "first pair", a.Get("response.result").String())
			require.Len(t, a.Get("parts").Array(), 2)
			require.Equal(t, "image/png", a.Get("parts.0.inlineData.mimeType").String())
			require.Equal(t, "AQID", a.Get("parts.0.inlineData.data").String())
			require.Equal(t, "image/webp", a.Get("parts.1.inlineData.mimeType").String())
			require.Equal(t, "BAUG", a.Get("parts.1.inlineData.data").String())
			require.Len(t, b.Get("parts").Array(), 1)
			require.Equal(t, "image/jpeg", b.Get("parts.0.inlineData.mimeType").String())
			require.Equal(t, "BwgJ", b.Get("parts.0.inlineData.data").String())
		})
	}
}

func TestToolResultTextAndOlderModelWireUnchanged(t *testing.T) {
	for _, model := range []string{"gemini-3.8-flash", "gemini-2.5-flash", "claude-sonnet-4-6"} {
		t.Run(model, func(t *testing.T) {
			req := &ClaudeRequest{Model: model, Messages: []ClaudeMessage{
				{Role: "assistant", Content: json.RawMessage(`[{"type":"tool_use","id":"call_1","name":"read","input":{}}]`)},
				{Role: "user", Content: json.RawMessage(`[{"type":"tool_result","tool_use_id":"call_1","content":"plain text"}]`)},
			}}
			wire, err := TransformClaudeToGeminiWithOptions(req, "test-project", model, TransformOptions{})
			require.NoError(t, err)
			fr := gjson.GetBytes(wire, "request.contents.1.parts.0.functionResponse")
			require.Equal(t, "plain text", fr.Get("response.result").String())
			require.False(t, fr.Get("parts").Exists())
		})
	}
}

func TestToolResultMediaPlacement(t *testing.T) {
	for _, model := range []string{"gemini-3-flash", "gemini-4-argon", "gemini-2.5-flash", "claude-sonnet-4-6"} {
		t.Run(model, func(t *testing.T) {
			req := &ClaudeRequest{Messages: []ClaudeMessage{
				{Role: "assistant", Content: json.RawMessage(`[{"type":"tool_use","id":"call_1","name":"read","input":{}}]`)},
				{Role: "user", Content: json.RawMessage(`[{"type":"tool_result","tool_use_id":"call_1","content":[
 {"type":"image","source":{"type":"base64","media_type":"image/png","data":"AQID"}},
 {"type":"document","source":{"type":"base64","media_type":"application/pdf","data":"BAUG"}},
 {"type":"image","source":{"type":"base64","media_type":"image/png","data":""}}
 ]}]`)},
			}}
			wire, err := TransformClaudeToGeminiWithOptions(req, "offline-project", model, TransformOptions{})
			require.NoError(t, err)
			parts := gjson.GetBytes(wire, "request.contents.1.parts").Array()
			nested := model == "gemini-3-flash" || model == "gemini-4-argon"
			var media []gjson.Result
			if nested {
				require.Len(t, parts, 1)
				media = parts[0].Get("functionResponse.parts").Array()
			} else {
				require.Len(t, parts, 3)
				require.False(t, parts[0].Get("functionResponse.parts").Exists())
				media = parts[1:]
			}
			require.Len(t, media, 2)
			require.Equal(t, "AQID", media[0].Get("inlineData.data").String())
			require.Equal(t, "application/pdf", media[1].Get("inlineData.mimeType").String())
			require.Equal(t, "BAUG", media[1].Get("inlineData.data").String())
		})
	}
}
