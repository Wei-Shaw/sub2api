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

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func zhipuResponsesAccount() *Account {
	return &Account{ID: 42, Platform: PlatformZhipu, Type: AccountTypeAPIKey, Status: StatusActive,
		Credentials: map[string]any{"api_key": "sk-test", "account_mode": AccountModeCoding, "api_protocol": APIProtocolResponses},
		// A previously adaptive account can still have a stale negative probe result.
		Extra: map[string]any{"openai_responses_supported": false},
	}
}

func TestZhipuResponsesExplicitOptIn(t *testing.T) {
	for _, mode := range []string{"", AccountModePayG, AccountModeCoding} {
		for _, protocol := range []string{"", APIProtocolAdaptive, APIProtocolChatCompletions, APIProtocolAnthropic, APIProtocolResponses} {
			t.Run(mode+"/"+protocol, func(t *testing.T) {
				a := zhipuResponsesAccount()
				a.Credentials["account_mode"], a.Credentials["api_protocol"] = mode, protocol
				wantNative := mode == AccountModeCoding && protocol == APIProtocolResponses
				require.Equal(t, mode == AccountModeCoding, a.SupportsNativeCNResponses())
				require.Equal(t, wantNative, a.UsesNativeCNResponses())
				if protocol != APIProtocolAnthropic {
					require.Equal(t, !wantNative, shouldForwardOpenAIResponsesViaRawChatCompletions(a))
				}
			})
		}
	}
	a := zhipuResponsesAccount()
	require.Equal(t, DefaultZhipuResponsesBaseURL, a.GetOpenAIBaseURL())
	require.Equal(t, DefaultZhipuResponsesBaseURL, a.GetCNProtocolBaseURL(APIProtocolResponses))
	a.Credentials["base_url"] = "https://relay.example/api/v1"
	require.Equal(t, "https://relay.example/api/v1", a.GetOpenAIBaseURL())
}

func TestZhipuResponsesForwardNativeJSONAndToolReplay(t *testing.T) {
	gin.SetMode(gin.TestMode)
	// The second turn supplies a complete client-managed tool history, as Codex does.
	inputs := []string{
		`[{"role":"user","content":[{"type":"input_text","text":"Call ping"}]}]`,
		`[{"type":"function_call","call_id":"call_ping","name":"ping","arguments":"{}"},{"type":"function_call_output","call_id":"call_ping","output":"pong"}]`,
	}
	for _, input := range inputs {
		body := []byte(`{"model":"glm-5.3","input":` + input + `,"stream":false,"store":false,"tools":[{"type":"function","name":"ping","parameters":{"type":"object","properties":{}}}]}`)
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
		upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: http.StatusOK,
			Header: http.Header{"Content-Type": []string{"application/json"}},
			Body:   io.NopCloser(strings.NewReader(`{"id":"resp_glm","object":"response","model":"glm-5.3","status":"completed","output":[{"type":"function_call","call_id":"call_ping","name":"ping","arguments":"{}"}],"usage":{"input_tokens":12,"output_tokens":4,"input_tokens_details":{"cached_tokens":3}}}`)),
		}}
		svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
		result, err := svc.Forward(context.Background(), c, zhipuResponsesAccount(), body)
		require.NoError(t, err)
		require.Equal(t, "https://open.bigmodel.cn/api/v1/responses", upstream.lastReq.URL.String())
		require.Equal(t, "Bearer sk-test", upstream.lastReq.Header.Get("Authorization"))
		require.JSONEq(t, input, gjson.GetBytes(upstream.lastBody, "input").Raw)
		require.False(t, gjson.GetBytes(upstream.lastBody, "messages").Exists())
		require.Equal(t, "function", gjson.GetBytes(upstream.lastBody, "tools.0.type").String())
		require.Equal(t, "call_ping", gjson.Get(rec.Body.String(), "output.0.call_id").String())
		require.Equal(t, 12, result.Usage.InputTokens)
		require.Equal(t, 4, result.Usage.OutputTokens)
		require.Equal(t, 3, result.Usage.CacheReadInputTokens)
	}
}

func TestZhipuResponsesForwardNativeStream(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"glm-5.3","input":"hello","stream":true,"store":false}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	stream := "event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"pong\"}\n\nevent: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_glm\",\"status\":\"completed\",\"usage\":{\"input_tokens\":12,\"output_tokens\":4}}}\n\n"
	upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: http.StatusOK,
		Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(stream)),
	}}
	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
	result, err := svc.Forward(context.Background(), c, zhipuResponsesAccount(), body)
	require.NoError(t, err)
	require.Equal(t, "https://open.bigmodel.cn/api/v1/responses", upstream.lastReq.URL.String())
	require.Contains(t, rec.Body.String(), `"delta":"pong"`)
	require.Contains(t, rec.Body.String(), `"type":"response.completed"`)
	require.True(t, result.Stream)
	require.Equal(t, 12, result.Usage.InputTokens)
	require.Equal(t, 4, result.Usage.OutputTokens)
}

func TestZhipuResponsesDoesNotApplyOtherProvidersStatePolicy(t *testing.T) {
	body := []byte(`{"model":"glm-5.3","input":"next","store":true,"previous_response_id":"resp_previous"}`)
	require.JSONEq(t, string(body), string(normalizeDeepSeekResponsesRequestBody(zhipuResponsesAccount(), body)))
}

func TestZhipuResponsesAccountTestIgnoresStaleNegativeProbe(t *testing.T) {
	account := zhipuResponsesAccount()
	svc, upstream := adaptiveCNAccountTestService(account, adaptiveCNResponsesTestResponse())
	c, rec := newTestContext()
	err := svc.TestAccountConnection(c, account.ID, "glm-5.3", "hello", AccountTestModeDefault)
	require.NoError(t, err)
	require.Equal(t, "https://open.bigmodel.cn/api/v1/responses", upstream.lastReq.URL.String())
	require.Contains(t, rec.Body.String(), `"success":true`)
}

func TestZhipuCodingAdaptiveKeepsChatBridge(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, path := range []string{"/v1/responses", "/v1/chat/completions"} {
		t.Run(path, func(t *testing.T) {
			a := zhipuResponsesAccount()
			a.Credentials["api_protocol"] = APIProtocolAdaptive
			a.Extra["openai_responses_supported"] = true
			a.Extra["openai_responses_mode"] = "force_responses"
			body := []byte(`{"model":"glm-5.3","input":"hello","stream":false}`)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
			upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: http.StatusOK,
				Header: http.Header{"Content-Type": []string{"application/json"}},
				Body:   io.NopCloser(strings.NewReader(`{"id":"chat_glm","object":"chat.completion","model":"glm-5.3","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)),
			}}
			svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
			var err error
			if path == "/v1/responses" {
				_, err = svc.Forward(context.Background(), c, a, body)
			} else {
				_, err = svc.ForwardAsChatCompletions(context.Background(), c, a, body, "", "")
			}
			require.NoError(t, err)
			require.Equal(t, "https://open.bigmodel.cn/api/coding/paas/v4/chat/completions", upstream.lastReq.URL.String())
			require.Equal(t, "hello", gjson.GetBytes(upstream.lastBody, "messages.0.content").String())
			require.False(t, gjson.GetBytes(upstream.lastBody, "input").Exists())
		})
	}
}
