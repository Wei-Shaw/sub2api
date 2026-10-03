package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOpenAIGatewayService_OAuthPassthrough_PreservesClientResponseFraming(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// The terminal output is empty even though the stream contains content.
	// A JSON caller must reach the existing SSE-to-JSON reconstruction path.
	upstreamSSE := strings.Join([]string{
		`data: {"type":"response.created","response":{"id":"resp_framing","object":"response","model":"gpt-5.2","status":"in_progress","output":[]}}`,
		"",
		`data: {"type":"response.output_item.added","output_index":0,"item":{"id":"msg_framing","type":"message","role":"assistant","status":"in_progress","content":[]}}`,
		"",
		`data: {"type":"response.content_part.added","item_id":"msg_framing","output_index":0,"content_index":0,"part":{"type":"output_text","text":"","annotations":[]}}`,
		"",
		`data: {"type":"response.output_text.delta","item_id":"msg_framing","output_index":0,"content_index":0,"delta":"compaction checkpoint","sequence_number":429}`,
		"",
		`data: {"type":"response.completed","response":{"id":"resp_framing","object":"response","model":"gpt-5.2","status":"completed","error":null,"output":[],"usage":{"input_tokens":17,"output_tokens":5,"input_tokens_details":{"cached_tokens":9}}}}`,
		"",
		"data: [DONE]",
		"",
	}, "\n")

	for _, tc := range []struct {
		name        string
		streamField string
		stream      bool
	}{
		{name: "non_streaming", streamField: `,"stream":false`},
		{name: "stream_omitted"},
		{name: "streaming", streamField: `,"stream":true`, stream: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			body := []byte(fmt.Sprintf(`{"model":"gpt-5.2","store":true,"instructions":"test instructions","input":"summarize"%s}`, tc.streamField))
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
			c.Request.Header.Set("User-Agent", "codex_cli_rs/0.144.1")
			upstream := &httpUpstreamRecorder{resp: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "X-Request-Id": []string{"rid_framing"}},
				Body:       io.NopCloser(strings.NewReader(upstreamSSE)),
			}}
			svc := &OpenAIGatewayService{
				cfg:          &config.Config{Gateway: config.GatewayConfig{ForceCodexCLI: false}},
				httpUpstream: upstream,
			}
			account := &Account{
				ID:             123,
				Name:           "test",
				Platform:       PlatformOpenAI,
				Type:           AccountTypeOAuth,
				Concurrency:    1,
				Credentials:    map[string]any{"access_token": "oauth-token", "chatgpt_account_id": "test-account"},
				Extra:          map[string]any{"openai_passthrough": true, "openai_oauth_responses_websockets_v2_mode": OpenAIWSIngressModeOff},
				Status:         StatusActive,
				Schedulable:    true,
				RateMultiplier: f64p(1),
			}

			result, err := svc.Forward(context.Background(), c, account, body)
			require.NoError(t, err)
			require.NotNil(t, result)
			require.Equal(t, tc.stream, result.Stream)
			require.True(t, gjson.GetBytes(upstream.lastBody, "stream").Bool(), "OAuth upstream still requires SSE")
			require.False(t, gjson.GetBytes(upstream.lastBody, "store").Bool())
			require.Equal(t, http.StatusOK, rec.Code)
			require.Equal(t, "resp_framing", result.ResponseID)
			require.EqualValues(t, 17, result.Usage.InputTokens)
			require.EqualValues(t, 5, result.Usage.OutputTokens)
			require.EqualValues(t, 9, result.Usage.CacheReadInputTokens)

			if tc.stream {
				require.Contains(t, rec.Header().Get("Content-Type"), "text/event-stream")
				// The streaming handler may omit the redundant [DONE] sentinel
				// after response.completed; every response event must survive.
				responseEvents := func(body string) []string {
					var events []string
					for _, line := range strings.Split(body, "\n") {
						if data, ok := strings.CutPrefix(line, "data: "); ok && data != "[DONE]" {
							events = append(events, data)
						}
					}
					return events
				}
				require.Equal(t, responseEvents(upstreamSSE), responseEvents(rec.Body.String()))
			} else {
				require.Contains(t, rec.Header().Get("Content-Type"), "application/json")
				require.True(t, gjson.ValidBytes(rec.Body.Bytes()), "non-streaming clients require a JSON response")
				require.Equal(t, "resp_framing", gjson.GetBytes(rec.Body.Bytes(), "id").String())
				require.Equal(t, "completed", gjson.GetBytes(rec.Body.Bytes(), "status").String())
				require.Equal(t, "compaction checkpoint", gjson.GetBytes(rec.Body.Bytes(), "output.0.content.0.text").String())
			}
		})
	}
}
