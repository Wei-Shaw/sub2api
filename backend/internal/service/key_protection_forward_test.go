//go:build unit

package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/keyprotection"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// This exercises actual gateway forwarding and both protocol conversion
// directions, with a local in-memory upstream double. Fixtures are fictional.
func TestKeyProtectionActualGatewayForwarding(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const secret = "ghp_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	for _, protocol := range []string{"chat", "responses", "messages"} {
		for _, stream := range []bool{false, true} {
			name := protocol + "/json"
			if stream {
				name = protocol + "/stream"
			}
			t.Run(name, func(t *testing.T) {
				streamText := "false"
				if stream {
					streamText = "true"
				}
				body := `{"model":"gpt-5.4","stream":` + streamText + `,"messages":[{"role":"user","content":"Use ` + secret + `"}]}`
				path := "/v1/chat/completions"
				if protocol == "responses" {
					path = "/v1/responses"
					body = `{"model":"gpt-5.4","stream":` + streamText + `,"input":"Use ` + secret + `"}`
				}
				if protocol == "messages" {
					path = "/v1/messages"
					body = `{"model":"gpt-5.4","max_tokens":256,"stream":` + streamText + `,"messages":[{"role":"user","content":"Use ` + secret + `"}]}`
				}
				state, err := keyprotection.NewState(keyprotection.DefaultConfig())
				require.NoError(t, err)
				protected, err := state.ProtectJSON([]byte(body), protocol)
				require.NoError(t, err)
				token, err := state.ProtectText(secret)
				require.NoError(t, err)
				require.NotEmpty(t, token)
				response := `{"id":"chatcmpl_fixture","object":"chat.completion","model":"gpt-5.4","choices":[{"index":0,"message":{"role":"assistant","content":"Use ` + token + `","tool_calls":[{"id":"call_fixture","type":"function","function":{"name":"lookup","arguments":"{\"key\":\"` + token + `\"}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":11,"completion_tokens":7,"total_tokens":18}}`
				contentType := "application/json"
				if stream {
					contentType = "text/event-stream"
					response = "data: " + `{"id":"chatcmpl_fixture","object":"chat.completion.chunk","model":"gpt-5.4","choices":[{"index":0,"delta":{"role":"assistant","content":"Use ` + token + `"}}]}` + "\n\n" +
						"data: " + `{"id":"chatcmpl_fixture","object":"chat.completion.chunk","model":"gpt-5.4","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_fixture","type":"function","function":{"name":"lookup","arguments":"{\"key\":\"` + token + `\"}"}}]}}]}` + "\n\n" +
						"data: " + `{"id":"chatcmpl_fixture","object":"chat.completion.chunk","model":"gpt-5.4","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":11,"completion_tokens":7,"total_tokens":18}}` + "\n\n" +
						"data: [DONE]\n\n"
				}
				upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{contentType}}, Body: io.NopCloser(strings.NewReader(response))}}
				svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
				recorder := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(recorder)
				ctx := keyprotection.WithProtected(context.Background())
				c.Request = httptest.NewRequest(http.MethodPost, path, bytes.NewReader(protected)).WithContext(ctx)
				c.Request.Header.Set("Content-Type", "application/json")
				c.Set(keyprotection.ProtectedGinKey, true)
				writer := keyprotection.NewResponseWriter(c.Writer, state, protocol)
				c.Writer = writer
				var result *OpenAIForwardResult
				switch protocol {
				case "chat":
					result, err = svc.forwardAsRawChatCompletions(ctx, c, rawChatCompletionsTestAccount(), protected, "")
				case "responses":
					result, err = svc.Forward(ctx, c, forceChatResponsesFallbackAccount(), protected)
				case "messages":
					result, err = svc.ForwardAsAnthropic(ctx, c, forceChatMessagesFallbackAccount(), protected, "", "")
				}
				require.NoError(t, err)
				require.NoError(t, writer.Finish())
				require.NotNil(t, upstream.lastReq)
				require.NotContains(t, string(upstream.lastBody), secret, "actual upstream request leaked fixture")
				require.Contains(t, string(upstream.lastBody), token, "protocol conversion lost placeholder")
				require.NotContains(t, recorder.Body.String(), token, "client text or tool arguments were not restored")
				require.GreaterOrEqual(t, strings.Count(recorder.Body.String(), secret), 2, "text and tool parameter need original credential")
				require.NotNil(t, result)
				require.Equal(t, 11, result.Usage.InputTokens)
				require.Equal(t, 7, result.Usage.OutputTokens)
			})
		}
	}
}

func TestKeyProtectionForcesHTTPForWSConfiguredAccount(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const secret = "ghp_BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB"
	state, err := keyprotection.NewState(keyprotection.DefaultConfig())
	require.NoError(t, err)
	body, err := state.ProtectJSON([]byte(`{"model":"gpt-5.4","input":"`+secret+`","stream":false}`), "responses")
	require.NoError(t, err)
	token, err := state.ProtectText(secret)
	require.NoError(t, err)
	upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"id":"resp_protected_http","object":"response","status":"completed","model":"gpt-5.4","output":[{"id":"msg_fixture","type":"message","role":"assistant","content":[{"type":"output_text","text":"` + token + `"}]}],"usage":{"input_tokens":11,"output_tokens":7,"total_tokens":18}}`))}}
	cfg := rawChatCompletionsTestConfig()
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.APIKeyEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
	account := rawChatCompletionsTestAccount()
	account.Extra = map[string]any{"responses_websockets_v2_enabled": true}
	svc := &OpenAIGatewayService{cfg: cfg, httpUpstream: upstream, openaiWSResolver: NewOpenAIWSProtocolResolver(cfg)}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	ctx := keyprotection.WithProtected(context.Background())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body)).WithContext(ctx)
	writer := keyprotection.NewResponseWriter(c.Writer, state, "responses")
	c.Writer = writer
	result, err := svc.Forward(ctx, c, account, body)
	require.NoError(t, err)
	require.NoError(t, writer.Finish())
	require.NotNil(t, result)
	require.NotNil(t, upstream.lastReq)
	require.Equal(t, "key_protection_http", c.GetString("openai_ws_transport_reason"))
	require.Equal(t, "http://upstream.example/v1/responses", upstream.lastReq.URL.String())
	require.NotContains(t, string(upstream.lastBody), secret)
	require.Contains(t, string(upstream.lastBody), token)
	require.Contains(t, recorder.Body.String(), secret)
}
