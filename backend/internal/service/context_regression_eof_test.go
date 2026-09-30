//go:build unit

package service

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// A transport EOF is a faulty upstream fixture, not evidence of a live failure.
func TestContextRegression_EarlyEOFIsVisibleToAnthropicClient(t *testing.T) {
	for _, keepalive := range []bool{false, true} {
		for _, tool := range []bool{false, true} {
			name := "text"
			if tool {
				name = "tool"
			}
			if keepalive {
				name += "_keepalive"
			}
			t.Run(name, func(t *testing.T) {
				gin.SetMode(gin.TestMode)
				body := []byte(`{"model":"gpt-6.1-sol","max_tokens":256,"stream":true,"messages":[{"role":"user","content":"continue task"}]}`)
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
				stream := `data: {"type":"response.created","response":{"id":"resp_eof","model":"gpt-6.1-sol","status":"in_progress","output":[]}}` + "\n\n"
				if tool {
					stream += `data: {"type":"response.output_item.added","output_index":0,"item":{"id":"fc_eof","type":"function_call","name":"Edit","call_id":"call_eof","arguments":""}}` + "\n\n"
					stream += `data: {"type":"response.function_call_arguments.delta","output_index":0,"item_id":"fc_eof","delta":"{\"nonce\":\"partial"}` + "\n\n"
				} else {
					stream += `data: {"type":"response.output_text.delta","output_index":0,"content_index":0,"delta":"partial output"}` + "\n\n"
				}
				cfg := rawChatCompletionsTestConfig()
				if keepalive {
					cfg.Gateway.StreamKeepaliveInterval = 1
				}
				svc := &OpenAIGatewayService{cfg: cfg, httpUpstream: &httpUpstreamRecorder{resp: &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(stream))}}}
				_, err := svc.ForwardAsAnthropic(context.Background(), c, rawChatCompletionsTestAccount(), body, "", "")
				require.Error(t, err)
				require.Contains(t, err.Error(), "missing terminal event")
				require.NotContains(t, rec.Body.String(), "event: message_stop")
				// The error must reach the client, not only appear in server logs.
				require.Contains(t, rec.Body.String(), "event: error")
				found := false
				for _, line := range strings.Split(rec.Body.String(), "\n") {
					if !strings.HasPrefix(line, "data: ") {
						continue
					}
					var event map[string]any
					if json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event) == nil && event["type"] == "error" {
						found = true
					}
				}
				require.True(t, found, "Anthropic error event must contain valid JSON")
			})
		}
	}
}
