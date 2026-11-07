package service

import (
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

func TestOpenAICompatBufferedReasoningContent(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// A populated terminal output prevents the accumulator from reconstructing
	// the reasoning from deltas, as in vLLM's Responses stream (#8546/#8547).
	const stream = `data: {"type":"response.reasoning_text.delta","output_index":0,"delta":"think "}

data: {"type":"response.reasoning_text.delta","output_index":0,"delta":"more"}

data: {"type":"response.output_text.delta","output_index":1,"delta":"answer"}

data: {"type":"response.completed","response":{"id":"resp_reasoning","model":"reasoning-model","status":"completed","output":[{"type":"reasoning","summary":[],"content":[{"type":"reasoning_text","text":"think more"}],"encrypted_content":null,"status":"completed"},{"type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"answer"}]}],"usage":{"input_tokens":20,"output_tokens":15,"total_tokens":35,"output_tokens_details":{"reasoning_tokens":10}}}}

`
	for _, path := range []string{"/v1/chat/completions", "/v1/messages"} {
		t.Run(path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"model":"reasoning-model","stream":false}`))
			resp := &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "X-Request-Id": []string{"reasoning-rid"}},
				Body:       io.NopCloser(strings.NewReader(stream)),
			}
			account := &Account{ID: 40, Name: "responses-api-key", Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
			svc := &OpenAIGatewayService{}
			handle := svc.handleChatBufferedStreamingResponse
			if path == "/v1/messages" {
				handle = svc.handleAnthropicBufferedStreamingResponse
			}
			result, err := handle(resp, c, account, "reasoning-model", "reasoning-model", "reasoning-model", time.Now())
			require.NoError(t, err)
			require.NotNil(t, result)
			require.Equal(t, http.StatusOK, rec.Code)
			require.Contains(t, rec.Header().Get("Content-Type"), "application/json")
			require.Equal(t, "reasoning-rid", result.RequestID)
			require.False(t, result.Stream)
			require.Equal(t, 20, result.Usage.InputTokens)
			require.Equal(t, 15, result.Usage.OutputTokens)

			body := rec.Body.Bytes()
			if path == "/v1/chat/completions" {
				require.Equal(t, "think more", gjson.GetBytes(body, "choices.0.message.reasoning_content").String())
				require.Equal(t, "answer", gjson.GetBytes(body, "choices.0.message.content").String())
				require.Equal(t, "stop", gjson.GetBytes(body, "choices.0.finish_reason").String())
				require.EqualValues(t, 15, gjson.GetBytes(body, "usage.completion_tokens").Int())
				require.EqualValues(t, 10, gjson.GetBytes(body, "usage.completion_tokens_details.reasoning_tokens").Int())
			} else {
				require.EqualValues(t, 2, gjson.GetBytes(body, "content.#").Int())
				require.JSONEq(t, `{"type":"thinking","thinking":"think more","signature":""}`, gjson.GetBytes(body, "content.0").Raw)
				require.Equal(t, "answer", gjson.GetBytes(body, "content.1.text").String())
				require.Equal(t, "end_turn", gjson.GetBytes(body, "stop_reason").String())
				require.EqualValues(t, 15, gjson.GetBytes(body, "usage.output_tokens").Int())
			}
		})
	}
}
