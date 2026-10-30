package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestAntigravityResponsesToolResultImages(t *testing.T) {
	for _, model := range []string{"gemini-3.8-flash", "gemini-4-argon"} {
		t.Run(model, func(t *testing.T) {
			raw := []byte(`{"model":"client-alias","input":[
 {"type":"function_call","call_id":"call_A","name":"images","arguments":"{}"},
 {"type":"function_call","call_id":"call_B","name":"images","arguments":"{}"},
 {"type":"function_call_output","call_id":"call_A","output":[{"type":"input_text","text":"first pair"},{"type":"input_image","image_url":"data:image/png;base64,AQID"},{"type":"input_image","image_url":"data:image/webp;base64,BAUG"}]},
 {"type":"function_call_output","call_id":"call_B","output":[{"type":"input_image","image_url":"data:image/jpeg;base64,BwgJ"}]}
 ]}`)
			wire := toolImageCompatWire(t, raw, model)
			responses := []gjson.Result{}
			for _, content := range gjson.GetBytes(wire, "request.contents").Array() {
				for _, part := range content.Get("parts").Array() {
					if fr := part.Get("functionResponse"); fr.Exists() {
						responses = append(responses, fr)
					}
					require.False(t, part.Get("inlineData").Exists())
				}
			}
			require.Len(t, responses, 2)
			a, b := responses[0], responses[1]
			require.Equal(t, "images", a.Get("name").String())
			require.Equal(t, "images", b.Get("name").String())
			require.Equal(t, "first pair", a.Get("response.content").String())
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

func toolImageCompatWire(t *testing.T, raw []byte, model string) []byte {
	t.Helper()
	var req apicompat.ResponsesRequest
	require.NoError(t, json.Unmarshal(raw, &req))
	a, err := apicompat.ResponsesToAnthropicRequest(&req)
	require.NoError(t, err)
	body, err := json.Marshal(a)
	require.NoError(t, err)
	var claude antigravity.ClaudeRequest
	require.NoError(t, json.Unmarshal(body, &claude))
	service := &AntigravityGatewayService{}
	wire, err := service.buildAntigravityCompatGeminiBody(context.Background(), body, &claude, "offline-project", model)
	require.NoError(t, err)
	return wire
}
