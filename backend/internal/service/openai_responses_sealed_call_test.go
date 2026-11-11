package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOpenAIResponsesSealedFunctionCallRoundTrip(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const arguments = `{"message":"synthetic-sealed-message"}`
	for _, marker := range []struct {
		name             string
		raw              string
		encryptedContent string
	}{
		{name: "encrypted", raw: `["message"]`, encryptedContent: "synthetic-call-ciphertext"},
		{name: "plaintext", raw: `[]`},
		{name: "null", raw: `null`},
		{name: "absent", encryptedContent: "synthetic-call-ciphertext"},
	} {
		for _, shape := range []string{"terminal_output", "item_done", "delta_fallback"} {
			for _, mode := range []struct {
				name        string
				passthrough bool
				streaming   bool
			}{
				{name: "normal"},
				{name: "passthrough", passthrough: true},
				{name: "streaming", streaming: true},
			} {
				t.Run(marker.name+"/"+shape+"/"+mode.name, func(t *testing.T) {
					item := map[string]any{
						"type": "function_call", "call_id": "call_sealed", "name": "send_message",
						"namespace": "collaboration", "arguments": arguments,
					}
					if marker.raw != "" {
						item["encrypted_function_args"] = json.RawMessage(marker.raw)
					}
					if marker.encryptedContent != "" {
						item["encrypted_content"] = marker.encryptedContent
					}
					output := []any{}
					var frames []string
					addFrame := func(event map[string]any) {
						encoded, err := json.Marshal(event)
						require.NoError(t, err)
						frames = append(frames, "data: "+string(encoded)+"\n\n")
					}
					switch shape {
					case "terminal_output":
						output = append(output, item)
					case "item_done":
						addFrame(map[string]any{"type": "response.output_item.done", "output_index": 0, "item": item})
					case "delta_fallback":
						item["arguments"] = ""
						addFrame(map[string]any{"type": "response.output_item.added", "output_index": 0, "item": item})
						addFrame(map[string]any{"type": "response.function_call_arguments.delta", "output_index": 0, "delta": arguments})
					}
					addFrame(map[string]any{
						"type": "response.completed",
						"response": map[string]any{
							"id": "resp_sealed", "model": "test-model", "status": "completed", "output": output,
							"usage": map[string]int{"input_tokens": 7, "output_tokens": 3, "total_tokens": 10},
						},
					})

					rec := httptest.NewRecorder()
					c, _ := gin.CreateTestContext(rec)
					c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
					resp := &http.Response{
						StatusCode: http.StatusOK,
						Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
						Body:       io.NopCloser(strings.NewReader(strings.Join(frames, ""))),
					}
					account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
					svc := newCompactBridgeTestService()
					if mode.streaming {
						result, err := svc.handleStreamingResponse(context.Background(), resp, c, account, time.Now(), "test-model", "test-model")
						require.NoError(t, err)
						require.Equal(t, 7, result.usage.InputTokens)
					} else if mode.passthrough {
						result, err := svc.handleNonStreamingResponsePassthrough(context.Background(), resp, c, account, "test-model", "test-model")
						require.NoError(t, err)
						require.Equal(t, 7, result.usage.InputTokens)
					} else {
						result, err := svc.handleNonStreamingResponse(context.Background(), resp, c, account, "test-model", "test-model")
						require.NoError(t, err)
						require.Equal(t, 7, result.usage.InputTokens)
					}
					require.Equal(t, http.StatusOK, rec.Code)
					body := rec.Body.Bytes()
					if mode.streaming {
						require.Contains(t, rec.Header().Get("Content-Type"), "text/event-stream")
						_, terminal, ok := extractOpenAISSETerminalEvent(string(body))
						require.True(t, ok)
						body = []byte(gjson.GetBytes(terminal, "response").Raw)
					} else {
						require.Contains(t, rec.Header().Get("Content-Type"), "application/json")
					}
					require.Len(t, gjson.GetBytes(body, "output").Array(), 1)
					call := gjson.GetBytes(body, "output.0")
					require.Equal(t, "function_call", call.Get("type").String())
					require.Equal(t, "call_sealed", call.Get("call_id").String())
					require.Equal(t, "send_message", call.Get("name").String())
					require.Equal(t, arguments, call.Get("arguments").String())
					if marker.raw == "" {
						require.False(t, call.Get("encrypted_function_args").Exists())
					} else {
						require.Equal(t, marker.raw, call.Get("encrypted_function_args").Raw)
					}
					if marker.encryptedContent == "" {
						require.False(t, call.Get("encrypted_content").Exists())
					} else {
						require.Equal(t, marker.encryptedContent, call.Get("encrypted_content").String())
					}
					require.Equal(t, "collaboration", call.Get("namespace").String())
				})
			}
		}
	}
}
