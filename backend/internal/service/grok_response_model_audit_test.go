//go:build unit

package service

import (
	"context"
	"fmt"
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

func TestGrokResponseAuditNativeJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": {"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"id":"resp_audit","object":"response","model":"grok-4.5-build","output":[],"usage":{"input_tokens":2,"output_tokens":1}}`)),
	}}
	account := &Account{ID: 53, Platform: PlatformGrok, Type: AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "test-key", "base_url": "https://api.x.ai/v1"}}
	svc := &OpenAIGatewayService{httpUpstream: upstream}
	body := []byte(`{"model":"composer-2.5","input":"hi","stream":false}`)
	result, err := svc.forwardGrokResponses(context.Background(), c, account, body, "composer-2.5", false, time.Now())
	require.NoError(t, err)
	require.Equal(t, "grok-composer-2.5-fast", result.UpstreamModel)
	require.Equal(t, "grok-4.5-build", result.UpstreamResponseModel)
	require.Equal(t, "composer-2.5", result.Model)
	require.Contains(t, rec.Body.String(), `"model":"composer-2.5"`)
	require.True(t, *upstreamModelMismatch(result.UpstreamModel, result.UpstreamResponseModel))
}

func TestGrokResponseAuditNativeProtocols(t *testing.T) {
	cases := []struct {
		name, requested, returned string
		stream                    bool
		mismatchNil               bool
		mismatch                  bool
		clientModel               string
	}{
		{name: "native-sse", requested: "composer-2.5", returned: "grok-4.5-build", stream: true, mismatch: true, clientModel: "composer-2.5"},
		{name: "native-json", requested: "composer-2.5", returned: "grok-4.5-build", stream: false, mismatch: true, clientModel: "composer-2.5"},
		{name: "normal-alias", requested: "grok-4.7", returned: "grok-4.7-build", stream: true, clientModel: "grok-4.7-build"},
		{name: "missing-model", requested: "grok-4.7", returned: "", stream: false, mismatchNil: true},
	}
	for i, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			c, rec := grokAuditContext("/v1/responses")
			var body string
			contentType := "application/json"
			if tt.stream {
				contentType = "text/event-stream"
				body = grokAuditResponsesSSE("", tt.returned)
			} else {
				body = grokAuditResponsesJSON(tt.returned)
			}
			upstream := &httpUpstreamRecorder{resp: grokAuditHTTPResponse(contentType, body)}
			request := []byte(fmt.Sprintf(`{"model":%q,"input":"hi","stream":%t}`, tt.requested, tt.stream))
			result, err := (&OpenAIGatewayService{httpUpstream: upstream}).Forward(context.Background(), c, grokAuditAPIKeyAccount(88100+int64(i)), request)
			require.NoError(t, err)
			require.Equal(t, tt.requested, result.Model)
			require.Equal(t, tt.stream, result.Stream)
			require.Equal(t, tt.returned, result.UpstreamResponseModel)
			require.False(t, result.UpstreamResponseModelConflict)
			requireObservedMismatch(t, result, tt.mismatchNil, tt.mismatch)
			client := rec.Body.String()
			if tt.clientModel == "" {
				require.False(t, gjson.Get(client, "model").Exists())
				return
			}
			require.Contains(t, client, `"model":"`+tt.clientModel+`"`)
			if tt.returned != "" && tt.clientModel != tt.returned {
				require.NotContains(t, client, tt.returned)
			}
		})
	}
}

func TestGrokResponseAuditNativeNonStreamingSSE(t *testing.T) {
	c, rec := grokAuditContext("/v1/responses")
	upstream := &httpUpstreamRecorder{resp: grokAuditHTTPResponse("text/event-stream", grokAuditResponsesSSE("", "grok-4.5-build"))}
	request := []byte(`{"model":"composer-2.5","input":"hi","stream":false}`)
	result, err := (&OpenAIGatewayService{httpUpstream: upstream}).Forward(context.Background(), c, grokAuditAPIKeyAccount(88201), request)
	require.NoError(t, err)
	require.False(t, result.Stream)
	require.Equal(t, "grok-composer-2.5-fast", result.UpstreamModel)
	require.Equal(t, "grok-4.5-build", result.UpstreamResponseModel)
	require.False(t, result.UpstreamResponseModelConflict)
	require.True(t, *upstreamModelMismatch(result.UpstreamModel, result.UpstreamResponseModel))
	client := rec.Body.String()
	require.True(t, gjson.Valid(client), "non-streaming client should receive aggregated JSON, got %s", client)
	require.Equal(t, "composer-2.5", gjson.Get(client, "model").String())
	require.NotContains(t, client, "grok-4.5-build")
}

func TestGrokResponseAuditNativeEventConflict(t *testing.T) {
	for _, stream := range []bool{true, false} {
		t.Run(fmt.Sprintf("stream=%t", stream), func(t *testing.T) {
			c, rec := grokAuditContext("/v1/responses")
			upstream := &httpUpstreamRecorder{resp: grokAuditHTTPResponse("text/event-stream", grokAuditResponsesSSE("grok-4.5", "grok-4.7-build"))}
			request := []byte(fmt.Sprintf(`{"model":"composer-2.5","input":"hi","stream":%t}`, stream))
			result, err := (&OpenAIGatewayService{httpUpstream: upstream}).Forward(context.Background(), c, grokAuditAPIKeyAccount(88210), request)
			require.NoError(t, err)
			require.Equal(t, "grok-4.7-build", result.UpstreamResponseModel)
			require.True(t, result.UpstreamResponseModelConflict)
			client := rec.Body.String()
			require.Contains(t, client, `"model":"composer-2.5"`)
			require.NotContains(t, client, "grok-4.7-build")
			require.NotContains(t, client, "grok-4.5")
		})
	}
}

func TestGrokResponseAuditForwardDoesNotLeakPreviousObservation(t *testing.T) {
	c, rec := grokAuditContext("/v1/responses")
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		grokAuditHTTPResponse("application/json", grokAuditResponsesJSON("grok-4.5-build")),
		grokAuditHTTPResponse("application/json", grokAuditResponsesJSON("")),
	}}
	svc := &OpenAIGatewayService{httpUpstream: upstream}
	account := grokAuditAPIKeyAccount(88301)
	first, err := svc.Forward(context.Background(), c, account, []byte(`{"model":"grok-4.7","input":"hi","stream":false}`))
	require.NoError(t, err)
	require.Equal(t, "grok-4.5-build", first.UpstreamResponseModel)

	secondRec := grokAuditSwapWriter(c)
	second, err := svc.Forward(context.Background(), c, account, []byte(`{"model":"grok-4.7","input":"again","stream":false}`))
	require.NoError(t, err)
	require.Empty(t, second.UpstreamResponseModel)
	require.False(t, second.UpstreamResponseModelConflict)
	require.Nil(t, upstreamModelMismatch(second.UpstreamModel, second.UpstreamResponseModel))
	require.False(t, gjson.Get(secondRec.Body.String(), "model").Exists())
	require.NotContains(t, secondRec.Body.String(), "grok-4.5-build")
	require.Contains(t, rec.Body.String(), "grok-4.5-build")
}

func TestGrokResponseAuditRetryDoesNotKeepFailedAttemptModel(t *testing.T) {
	c, _ := grokAuditContext("/v1/responses")
	body := []byte(`{"model":"grok-4.7","previous_response_id":"resp_valid_history","input":[{"type":"reasoning","summary":[{"type":"summary_text","text":"keep this summary"}],"encrypted_content":"encrypted-reasoning"},{"type":"message","role":"user","content":[{"type":"input_text","text":"hi"}]}],"stream":false}`)
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		{
			StatusCode: http.StatusBadRequest,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"code":"invalid-argument","error":"Could not decrypt the provided encrypted_content. Ensure the value is unmodified.","model":"grok-failed-attempt"}`)),
		},
		grokAuditHTTPResponse("application/json", grokAuditResponsesJSON("grok-4.5-build")),
	}}
	result, err := (&OpenAIGatewayService{httpUpstream: upstream}).Forward(context.Background(), c, grokAuditAPIKeyAccount(88401), body)
	require.NoError(t, err)
	require.Len(t, upstream.requests, 2)
	require.Equal(t, "grok-4.5-build", result.UpstreamResponseModel)
	require.False(t, result.UpstreamResponseModelConflict)
}

func TestGrokResponseAuditAccountFailoverDoesNotKeepFailedAttemptModel(t *testing.T) {
	c, _ := grokAuditContext("/v1/responses")
	first := healthyGrokOAuthGatewayTestAccount(88501, "access-token-a")
	second := healthyGrokOAuthGatewayTestAccount(88502, "access-token-b")
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		{
			StatusCode: http.StatusServiceUnavailable,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"temporary"},"model":"grok-failed-attempt"}`)),
		},
		grokAuditHTTPResponse("application/json", grokAuditResponsesJSON("grok-4.7-build")),
	}}
	svc := grokAuditOAuthService(upstream, first, second)
	body := []byte(`{"model":"grok-4.7","input":"hi","stream":false}`)
	_, err := svc.Forward(context.Background(), c, first, body)
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)

	secondRec := grokAuditSwapWriter(c)
	result, err := svc.Forward(context.Background(), c, second, body)
	require.NoError(t, err)
	require.Equal(t, "grok-4.7-build", result.UpstreamResponseModel)
	require.False(t, result.UpstreamResponseModelConflict)
	require.NotContains(t, secondRec.Body.String(), "grok-failed-attempt")
}

func TestGrokResponseAuditChatRaw(t *testing.T) {
	tests := []struct {
		name   string
		stream bool
	}{
		{name: "json", stream: false},
		{name: "sse", stream: true},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, rec := grokAuditContext("/v1/chat/completions")
			var resp *http.Response
			if tt.stream {
				resp = grokAuditHTTPResponse("text/event-stream", grokAuditChatSSE("grok-4.5-build"))
			} else {
				resp = grokAuditHTTPResponse("application/json", grokAuditChatJSON("grok-4.5-build"))
			}
			upstream := &httpUpstreamRecorder{resp: resp}
			request := []byte(fmt.Sprintf(`{"model":"composer-2.5","messages":[{"role":"user","content":"hi"}],"stream":%t}`, tt.stream))
			svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
			result, err := svc.ForwardAsChatCompletions(context.Background(), c, grokAuditAPIKeyAccount(88600+int64(i)), request, "", "")
			require.NoError(t, err)
			require.Equal(t, "https://api.x.ai/v1/chat/completions", upstream.lastReq.URL.String())
			require.Equal(t, "grok-composer-2.5-fast", result.UpstreamModel)
			require.Equal(t, "grok-4.5-build", result.UpstreamResponseModel)
			require.False(t, result.UpstreamResponseModelConflict)
			require.True(t, *upstreamModelMismatch(result.UpstreamModel, result.UpstreamResponseModel))
			require.Contains(t, rec.Body.String(), `"model":"composer-2.5"`)
			require.NotContains(t, rec.Body.String(), "grok-4.5-build")
		})
	}
}

func TestGrokResponseAuditChatBridge(t *testing.T) {
	for i, stream := range []bool{false, true} {
		name := "aggregate"
		if stream {
			name = "stream"
		}
		t.Run(name, func(t *testing.T) {
			c, rec := grokAuditContext("/v1/chat/completions")
			account := healthyGrokOAuthGatewayTestAccount(88700+int64(i), "access-token")
			upstream := &httpUpstreamRecorder{resp: grokAuditHTTPResponse("text/event-stream", grokAuditResponsesSSE("", "grok-4.7-build"))}
			request := []byte(fmt.Sprintf(`{"model":"grok-4.7","messages":[{"role":"user","content":"hi"}],"stream":%t,"prompt_cache_key":"stable-session"}`, stream))
			result, err := grokAuditOAuthService(upstream, account).ForwardAsChatCompletions(context.Background(), c, account, request, "", "")
			require.NoError(t, err)
			require.Equal(t, grokChatResponsesEndpoint, result.UpstreamEndpoint)
			require.Contains(t, upstream.lastReq.URL.String(), "/responses")
			require.Equal(t, "grok-4.7", result.UpstreamModel)
			require.Equal(t, "grok-4.7-build", result.UpstreamResponseModel)
			require.False(t, result.UpstreamResponseModelConflict)
			mismatch := upstreamModelMismatch(result.UpstreamModel, result.UpstreamResponseModel)
			require.NotNil(t, mismatch)
			require.False(t, *mismatch)
			require.Contains(t, rec.Body.String(), `"model":"grok-4.7"`)
			require.NotContains(t, rec.Body.String(), "grok-4.7-build")
		})
	}
}

func TestGrokResponseAuditMessages(t *testing.T) {
	for i, stream := range []bool{false, true} {
		name := "aggregate"
		if stream {
			name = "stream"
		}
		t.Run(name, func(t *testing.T) {
			c, rec := grokAuditContext("/v1/messages")
			account := healthyGrokOAuthGatewayTestAccount(88800+int64(i), "access-token")
			upstream := &httpUpstreamRecorder{resp: grokAuditHTTPResponse("text/event-stream", grokAuditResponsesSSE("", "grok-4.7-build"))}
			request := []byte(fmt.Sprintf(`{"model":"grok-4.7","max_tokens":32,"stream":%t,"messages":[{"role":"user","content":"hi"}]}`, stream))
			result, err := grokAuditOAuthService(upstream, account).ForwardAsAnthropic(context.Background(), c, account, request, "", "")
			require.NoError(t, err)
			require.Contains(t, upstream.lastReq.URL.String(), "/responses")
			require.Equal(t, "grok-4.7", result.Model)
			require.Equal(t, "grok-4.7", result.UpstreamModel)
			require.Equal(t, "grok-4.7-build", result.UpstreamResponseModel)
			require.False(t, result.UpstreamResponseModelConflict)
			mismatch := upstreamModelMismatch(result.UpstreamModel, result.UpstreamResponseModel)
			require.NotNil(t, mismatch)
			require.False(t, *mismatch)
			client := rec.Body.String()
			if !stream {
				require.Equal(t, "grok-4.7", gjson.Get(client, "model").String())
			}
			require.NotContains(t, client, "grok-4.7-build")
		})
	}
}

func TestGrokResponseAuditConvertedStreamingConflict(t *testing.T) {
	sse := grokAuditResponsesSSE("grok-4.5", "grok-4.7-build")
	t.Run("chat-bridge", func(t *testing.T) {
		c, rec := grokAuditContext("/v1/chat/completions")
		account := healthyGrokOAuthGatewayTestAccount(88901, "access-token")
		upstream := &httpUpstreamRecorder{resp: grokAuditHTTPResponse("text/event-stream", sse)}
		request := []byte(`{"model":"grok-4.7","messages":[{"role":"user","content":"hi"}],"stream":true,"prompt_cache_key":"stable-session"}`)
		result, err := grokAuditOAuthService(upstream, account).ForwardAsChatCompletions(context.Background(), c, account, request, "", "")
		require.NoError(t, err)
		require.Equal(t, grokChatResponsesEndpoint, result.UpstreamEndpoint)
		require.Equal(t, "grok-4.7-build", result.UpstreamResponseModel)
		require.True(t, result.UpstreamResponseModelConflict)
		require.Contains(t, rec.Body.String(), `"model":"grok-4.7"`)
		require.NotContains(t, rec.Body.String(), "grok-4.7-build")
	})
	t.Run("messages", func(t *testing.T) {
		c, rec := grokAuditContext("/v1/messages")
		account := healthyGrokOAuthGatewayTestAccount(88902, "access-token")
		upstream := &httpUpstreamRecorder{resp: grokAuditHTTPResponse("text/event-stream", sse)}
		request := []byte(`{"model":"grok-4.7","max_tokens":32,"stream":true,"messages":[{"role":"user","content":"hi"}]}`)
		result, err := grokAuditOAuthService(upstream, account).ForwardAsAnthropic(context.Background(), c, account, request, "", "")
		require.NoError(t, err)
		require.Equal(t, "grok-4.7-build", result.UpstreamResponseModel)
		require.True(t, result.UpstreamResponseModelConflict)
		require.Equal(t, "grok-4.7", result.Model)
		require.NotContains(t, rec.Body.String(), "grok-4.7-build")
		require.NotContains(t, rec.Body.String(), "grok-4.5")
	})
}

func TestGrokResponseAuditConvertedAggregateConflict(t *testing.T) {
	sse := grokAuditResponsesSSE("grok-4.5", "grok-4.7-build")
	for i, entry := range []string{"chat-bridge", "messages"} {
		t.Run(entry, func(t *testing.T) {
			result, client := grokAuditRunConvertedAggregate(t, entry, 89001+int64(i), sse)
			require.False(t, result.Stream)
			require.Equal(t, "grok-4.7", result.Model)
			require.Equal(t, "grok-4.7", result.UpstreamModel)
			require.Equal(t, "grok-4.7-build", result.UpstreamResponseModel)
			require.True(t, result.UpstreamResponseModelConflict)
			requireObservedMismatch(t, result, false, false)
			require.True(t, gjson.Valid(client), "aggregate client should receive JSON, got %s", client)
			require.Equal(t, "grok-4.7", gjson.Get(client, "model").String())
			require.NotContains(t, client, "grok-4.7-build")
			require.NotContains(t, client, "grok-4.5")
		})
	}
}

func TestGrokResponseAuditConvertedAggregateModelBoundaries(t *testing.T) {
	cases := []struct {
		name, created, completed, wantModel string
		conflict, mismatchNil, mismatch     bool
	}{
		{name: "same", created: "grok-4.7-build", completed: "grok-4.7-build", wantModel: "grok-4.7-build"},
		{name: "missing", mismatchNil: true},
		{name: "created-without-terminal-model", created: "grok-4.5", wantModel: "grok-4.5", mismatch: true},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			sse := grokAuditResponsesSSE(tt.created, tt.completed)
			for i, entry := range []string{"chat-bridge", "messages"} {
				t.Run(entry, func(t *testing.T) {
					result, client := grokAuditRunConvertedAggregate(t, entry, 89100+int64(i), sse)
					require.False(t, result.Stream)
					require.Equal(t, tt.wantModel, result.UpstreamResponseModel)
					require.Equal(t, tt.conflict, result.UpstreamResponseModelConflict)
					requireObservedMismatch(t, result, tt.mismatchNil, tt.mismatch)
					require.True(t, gjson.Valid(client), "aggregate client should receive JSON, got %s", client)
					require.Equal(t, "grok-4.7", gjson.Get(client, "model").String())
					if tt.wantModel != "" && tt.wantModel != "grok-4.7" {
						require.NotContains(t, client, tt.wantModel)
					}
					if tt.created != "" && tt.created != tt.wantModel && tt.created != "grok-4.7" {
						require.NotContains(t, client, tt.created)
					}
				})
			}
		})
	}
}

func TestGrokResponseAuditConvertedAggregateKeepsNonGrokTerminalOnly(t *testing.T) {
	sse := grokAuditResponsesSSE("gpt-5.4-mini", "gpt-5.4")
	t.Run("chat", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		body := []byte(`{"model":"gpt-5.4","messages":[{"role":"user","content":"hi"}],"stream":false}`)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(string(body)))
		c.Request.Header.Set("Content-Type", "application/json")
		upstream := &httpUpstreamRecorder{resp: grokAuditHTTPResponse("text/event-stream", sse)}
		account := &Account{
			ID:          89201,
			Name:        "openai-oauth",
			Platform:    PlatformOpenAI,
			Type:        AccountTypeOAuth,
			Concurrency: 1,
			Credentials: map[string]any{
				"access_token":       "oauth-token",
				"chatgpt_account_id": "chatgpt-acc",
			},
		}
		result, err := (&OpenAIGatewayService{httpUpstream: upstream}).ForwardAsChatCompletions(context.Background(), c, account, body, "", "gpt-5.4")
		require.NoError(t, err)
		require.False(t, result.Stream)
		require.Equal(t, "gpt-5.4", result.UpstreamResponseModel)
		require.False(t, result.UpstreamResponseModelConflict)
		require.Equal(t, result.Model, gjson.Get(rec.Body.String(), "model").String())
		require.NotContains(t, rec.Body.String(), "gpt-5.4-mini")
	})
	t.Run("messages", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		body := []byte(`{"model":"gpt-5.4","max_tokens":32,"messages":[{"role":"user","content":"hi"}],"stream":false}`)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(string(body)))
		c.Request.Header.Set("Content-Type", "application/json")
		upstream := &httpUpstreamRecorder{resp: grokAuditHTTPResponse("text/event-stream", sse)}
		result, err := (&OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}).ForwardAsAnthropic(
			context.Background(), c, rawChatCompletionsTestAccount(), body, "", "",
		)
		require.NoError(t, err)
		require.False(t, result.Stream)
		require.Equal(t, "gpt-5.4", result.UpstreamResponseModel)
		require.False(t, result.UpstreamResponseModelConflict)
		require.Equal(t, result.Model, gjson.Get(rec.Body.String(), "model").String())
		require.NotContains(t, rec.Body.String(), "gpt-5.4-mini")
	})
}

func TestGrokResponseAuditAggregatePriorStateStaysBounded(t *testing.T) {
	const repeats = 48
	overlong := strings.Repeat("界", upstreamResponseModelMaxLength+50)
	late := "grok-late-distinct"
	wantModel := string([]rune(overlong)[:upstreamResponseModelMaxLength])
	grok := grokAuditAPIKeyAccount(89301)
	other := &Account{ID: 89302, Platform: PlatformOpenAI, Type: AccountTypeOAuth}

	t.Run("repeated-declarations", func(t *testing.T) {
		terminal, prior := grokAuditReadBufferedPriors(t, grok, grokAuditPriorModelSSE(repeats, overlong, late, overlong))
		require.Equal(t, wantModel, prior.observer.first)
		require.LessOrEqual(t, len([]rune(prior.observer.first)), upstreamResponseModelMaxLength)
		require.Empty(t, prior.observer.terminal)
		require.True(t, prior.observer.conflict)
		observer := &upstreamResponseModelObserver{}
		prior.apply(observer)
		observer.Observe(terminal, true)
		require.Equal(t, wantModel, observer.Model())
		require.True(t, observer.Conflict())
	})

	t.Run("overlong-model", func(t *testing.T) {
		_, prior := grokAuditReadBufferedPriors(t, grok, grokAuditPriorModelSSE(1, overlong, "", ""))
		require.Equal(t, wantModel, prior.observer.first)
		require.LessOrEqual(t, len([]rune(prior.observer.first)), upstreamResponseModelMaxLength)
		require.False(t, prior.observer.conflict)
	})

	t.Run("overlong-shared-prefix", func(t *testing.T) {
		diverged := wantModel + strings.Repeat("外", 50)
		_, prior := grokAuditReadBufferedPriors(t, grok, grokAuditPriorModelSSE(repeats, overlong, diverged, ""))
		require.Equal(t, wantModel, prior.observer.first)
		require.LessOrEqual(t, len([]rune(prior.observer.first)), upstreamResponseModelMaxLength)
		require.False(t, prior.observer.conflict)
	})

	t.Run("non-grok-skips-collection", func(t *testing.T) {
		terminal, prior := grokAuditReadBufferedPriors(t, other, grokAuditPriorModelSSE(repeats, "gpt-5.4-mini", "gpt-5.4-nano", "gpt-5.4"))
		require.False(t, prior.enabled)
		require.Empty(t, prior.observer.first)
		require.False(t, prior.observer.conflict)
		observer := &upstreamResponseModelObserver{}
		prior.apply(observer)
		observer.Observe(terminal, true)
		require.Equal(t, "gpt-5.4", observer.Model())
		require.False(t, observer.Conflict())
	})

	t.Run("entries", func(t *testing.T) {
		sse := grokAuditPriorModelSSE(repeats, overlong, late, overlong)
		for i, entry := range []string{"chat-bridge", "messages"} {
			t.Run(entry, func(t *testing.T) {
				result, client := grokAuditRunConvertedAggregate(t, entry, 89310+int64(i), sse)
				require.False(t, result.Stream)
				require.Equal(t, wantModel, result.UpstreamResponseModel)
				require.True(t, result.UpstreamResponseModelConflict)
				require.Equal(t, "grok-4.7", gjson.Get(client, "model").String())
				require.NotContains(t, client, late)
				require.NotContains(t, client, wantModel)
			})
		}
	})
}

func grokAuditReadBufferedPriors(t *testing.T, account *Account, sse string) (string, openAICompatPriorModelAudit) {
	t.Helper()
	c, _ := grokAuditContext("/v1/messages")
	resp := grokAuditHTTPResponse("text/event-stream", sse)
	response, _, _, prior, err := (&OpenAIGatewayService{}).readOpenAICompatBufferedTerminal(resp, c, account, "openai messages buffered", "req-bound", nil)
	require.NoError(t, err)
	require.NotNil(t, response)
	return response.Model, prior
}

func grokAuditPriorModelSSE(repeats int, repeated, late, terminal string) string {
	var b strings.Builder
	for i := 0; i < repeats; i++ {
		if repeated == "" {
			break
		}
		fmt.Fprintf(&b, "data: {\"type\":\"response.in_progress\",\"response\":{\"id\":\"resp_audit\",\"model\":%q,\"output\":[]}}\n\n", repeated)
	}
	if late != "" {
		fmt.Fprintf(&b, "data: {\"type\":\"response.in_progress\",\"response\":{\"id\":\"resp_audit\",\"model\":%q,\"output\":[]}}\n\n", late)
	}
	b.WriteString("data: {\"type\":\"response.output_text.delta\",\"delta\":\"ok\"}\n\n")
	completed := `{"id":"resp_audit","object":"response","status":"completed","output":[{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"ok"}]}],"usage":{"input_tokens":2,"output_tokens":1}}`
	if terminal != "" {
		completed = fmt.Sprintf(`{"id":"resp_audit","object":"response","model":%q,"status":"completed","output":[{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"ok"}]}],"usage":{"input_tokens":2,"output_tokens":1}}`, terminal)
	}
	b.WriteString("data: {\"type\":\"response.completed\",\"response\":" + completed + "}\n\n")
	return b.String()
}

func grokAuditRunConvertedAggregate(t *testing.T, entry string, id int64, sse string) (*OpenAIForwardResult, string) {
	t.Helper()
	switch entry {
	case "chat-bridge":
		c, rec := grokAuditContext("/v1/chat/completions")
		account := healthyGrokOAuthGatewayTestAccount(id, "access-token")
		upstream := &httpUpstreamRecorder{resp: grokAuditHTTPResponse("text/event-stream", sse)}
		request := []byte(`{"model":"grok-4.7","messages":[{"role":"user","content":"hi"}],"stream":false,"prompt_cache_key":"stable-session"}`)
		result, err := grokAuditOAuthService(upstream, account).ForwardAsChatCompletions(context.Background(), c, account, request, "", "")
		require.NoError(t, err)
		require.Equal(t, grokChatResponsesEndpoint, result.UpstreamEndpoint)
		require.Contains(t, upstream.lastReq.URL.String(), "/responses")
		return result, rec.Body.String()
	case "messages":
		c, rec := grokAuditContext("/v1/messages")
		account := healthyGrokOAuthGatewayTestAccount(id, "access-token")
		upstream := &httpUpstreamRecorder{resp: grokAuditHTTPResponse("text/event-stream", sse)}
		request := []byte(`{"model":"grok-4.7","max_tokens":32,"stream":false,"messages":[{"role":"user","content":"hi"}]}`)
		result, err := grokAuditOAuthService(upstream, account).ForwardAsAnthropic(context.Background(), c, account, request, "", "")
		require.NoError(t, err)
		require.Contains(t, upstream.lastReq.URL.String(), "/responses")
		return result, rec.Body.String()
	default:
		t.Fatalf("unknown aggregate entry %s", entry)
		return nil, ""
	}
}

func grokAuditSwapWriter(c *gin.Context) *httptest.ResponseRecorder {
	next, rec := grokAuditContext("/v1/responses")
	c.Writer = next.Writer
	return rec
}

func grokAuditContext(path string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, path, nil)
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("api_key", &APIKey{ID: 88000})
	return c, rec
}

func grokAuditAPIKeyAccount(id int64) *Account {
	return &Account{
		ID:          id,
		Name:        "grok-api-key",
		Platform:    PlatformGrok,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 2,
		Credentials: map[string]any{"api_key": "test-key", "base_url": "https://api.x.ai/v1"},
	}
}

func grokAuditOAuthService(upstream *httpUpstreamRecorder, accounts ...*Account) *OpenAIGatewayService {
	byID := make(map[int64]*Account, len(accounts))
	for _, account := range accounts {
		byID[account.ID] = account
	}
	repo := &grokQuotaAccountRepo{mockAccountRepoForPlatform: &mockAccountRepoForPlatform{accountsByID: byID}}
	return &OpenAIGatewayService{
		cfg:               rawChatCompletionsTestConfig(),
		httpUpstream:      upstream,
		grokTokenProvider: NewGrokTokenProvider(repo, nil),
		accountRepo:       repo,
	}
}

func grokAuditHTTPResponse(contentType, body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{contentType}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func grokAuditResponsesJSON(model string) string {
	if model == "" {
		return `{"id":"resp_audit","object":"response","status":"completed","output":[],"usage":{"input_tokens":2,"output_tokens":1}}`
	}
	return fmt.Sprintf(`{"id":"resp_audit","object":"response","model":%q,"status":"completed","output":[],"usage":{"input_tokens":2,"output_tokens":1}}`, model)
}

func grokAuditResponsesSSE(createdModel, completedModel string) string {
	lines := make([]string, 0, 8)
	if createdModel != "" {
		lines = append(lines, fmt.Sprintf(`data: {"type":"response.created","response":{"id":"resp_audit","model":%q,"output":[]}}`, createdModel), "")
	}
	lines = append(lines, `data: {"type":"response.output_text.delta","sequence_number":0,"delta":"ok"}`, "")
	completed := `{"id":"resp_audit","object":"response","status":"completed","output":[{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"ok"}]}],"usage":{"input_tokens":2,"output_tokens":1}}`
	if completedModel != "" {
		completed = fmt.Sprintf(`{"id":"resp_audit","object":"response","model":%q,"status":"completed","output":[{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"ok"}]}],"usage":{"input_tokens":2,"output_tokens":1}}`, completedModel)
	}
	lines = append(lines, `data: {"type":"response.completed","sequence_number":1,"response":`+completed+`}`, "")
	return strings.Join(lines, "\n")
}

func grokAuditChatJSON(model string) string {
	return fmt.Sprintf(`{"id":"chatcmpl_audit","object":"chat.completion","model":%q,"choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":1,"total_tokens":3}}`, model)
}

func grokAuditChatSSE(model string) string {
	return strings.Join([]string{
		fmt.Sprintf(`data: {"id":"chatcmpl_audit","object":"chat.completion.chunk","model":%q,"choices":[{"index":0,"delta":{"content":"ok"}}]}`, model),
		"",
		fmt.Sprintf(`data: {"id":"chatcmpl_audit","object":"chat.completion.chunk","model":%q,"choices":[],"usage":{"prompt_tokens":2,"completion_tokens":1,"total_tokens":3}}`, model),
		"",
		"data: [DONE]",
		"",
	}, "\n")
}

func requireObservedMismatch(t *testing.T, result *OpenAIForwardResult, wantNil, want bool) {
	t.Helper()
	got := upstreamModelMismatch(result.UpstreamModel, result.UpstreamResponseModel)
	if wantNil {
		require.Nil(t, got)
		return
	}
	require.NotNil(t, got)
	require.Equal(t, want, *got)
}
