//go:build unit

package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// publicModelRoute serves requests for public-model with claude-opus-5-5.
func publicModelRoute() CompositeModelRoute {
	return CompositeModelRoute{
		ID:             1,
		GroupID:        11,
		PublicModel:    "public-model",
		MatchType:      CompositeRouteMatchExact,
		TargetPlatform: PlatformAnthropic,
		UpstreamModel:  "claude-opus-5-5",
		Endpoint:       CompositeRouteEndpointAny,
		Priority:       100,
		Enabled:        true,
	}
}

// routeCompositeBody resolves body's model against route and rewrites the
// model the way the composite middleware does before any handler runs.
func routeCompositeBody(t *testing.T, route CompositeModelRoute, endpoint string, body []byte) (context.Context, []byte) {
	t.Helper()
	resolver := NewCompositeRouteResolver(compositeRouteRepoStub{routes: []CompositeModelRoute{route}})
	decision, err := resolver.Resolve(context.Background(), route.GroupID, gjson.GetBytes(body, "model").String(), endpoint)
	require.NoError(t, err)
	require.True(t, decision.Matched)
	body, err = sjson.SetBytes(body, "model", decision.UpstreamModel)
	require.NoError(t, err)
	return WithCompositeRouteDecision(context.Background(), decision), body
}

func anthropicMessageStartModel(t *testing.T, sse string) string {
	t.Helper()
	for _, line := range strings.Split(sse, "\n") {
		data, ok := strings.CutPrefix(line, "data: ")
		if ok && gjson.Get(data, "type").String() == "message_start" {
			return gjson.Get(data, "message.model").String()
		}
	}
	t.Fatalf("no message_start event in stream:\n%s", sse)
	return ""
}

func TestCompositeRouteRestoresPublicModelOnAnthropicForward(t *testing.T) {
	gin.SetMode(gin.TestMode)
	accounts := map[string]func() *Account{
		"oauth":              newAnthropicOAuthAccountForPartialUsageTest,
		"apikey-passthrough": newAnthropicAPIKeyAccountForTest,
	}
	for accountName, newAccount := range accounts {
		for _, stream := range []bool{false, true} {
			t.Run(accountName+"/stream="+strconv.FormatBool(stream), func(t *testing.T) {
				client := []byte(`{"model":"public-model","max_tokens":16,"stream":` + strconv.FormatBool(stream) +
					`,"messages":[{"role":"user","content":"hi"}]}`)
				ctx, body := routeCompositeBody(t, publicModelRoute(), CompositeRouteEndpointMessages, client)

				// The upstream reports its own dated id: the client still sees
				// the public name it asked for.
				upstreamBody := `{"id":"msg_1","type":"message","role":"assistant","model":"claude-opus-5-5-20260901","content":[{"type":"text","text":"hi"}],"usage":{"input_tokens":1,"output_tokens":1}}`
				contentType := "application/json"
				if stream {
					upstreamBody = strings.Join([]string{
						`event: message_start`,
						`data: {"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","model":"claude-opus-5-5-20260901","content":[],"usage":{"input_tokens":1}}}`,
						"",
						`event: message_delta`,
						`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":1}}`,
						"",
						`event: message_stop`,
						`data: {"type":"message_stop"}`,
						"",
						"",
					}, "\n")
					contentType = "text/event-stream"
				}
				upstream := &anthropicHTTPUpstreamRecorder{resp: &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": []string{contentType}},
					Body:       io.NopCloser(strings.NewReader(upstreamBody)),
				}}
				svc := newForwardPartialUsageServiceForTest(upstream)

				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil).WithContext(ctx)
				parsed, err := ParseGatewayRequest(NewRequestBodyRef(body), PlatformAnthropic)
				require.NoError(t, err)

				_, err = svc.Forward(ctx, c, newAccount(), parsed)
				require.NoError(t, err)

				require.Equal(t, "claude-opus-5-5", gjson.GetBytes(upstream.lastBody, "model").String())
				if stream {
					require.Equal(t, "public-model", anthropicMessageStartModel(t, rec.Body.String()))
				} else {
					require.Equal(t, "public-model", gjson.GetBytes(rec.Body.Bytes(), "model").String())
					require.Equal(t, "hi", gjson.GetBytes(rec.Body.Bytes(), "content.0.text").String())
				}
			})
		}
	}
}

func TestCompositeRouteRestoresPublicModelOnResponsesBridge(t *testing.T) {
	gin.SetMode(gin.TestMode)
	route := publicModelRoute()
	route.TargetPlatform = PlatformOpenAI
	route.UpstreamModel = "gpt-5.6-sol"
	for _, stream := range []bool{false, true} {
		t.Run("stream="+strconv.FormatBool(stream), func(t *testing.T) {
			client := []byte(`{"model":"public-model","max_tokens":16,"stream":` + strconv.FormatBool(stream) +
				`,"messages":[{"role":"user","content":"hello"}]}`)
			ctx, body := routeCompositeBody(t, route, CompositeRouteEndpointMessages, client)

			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(string(body))).WithContext(ctx)
			c.Request.Header.Set("Content-Type", "application/json")
			upstream := &httpUpstreamRecorder{resp: openAICompatSSECompletedResponse("resp_route", route.UpstreamModel)}
			upstream.resp.Body = io.NopCloser(io.MultiReader(
				strings.NewReader(`data: {"type":"response.created","response":{"id":"resp_route","model":"`+route.UpstreamModel+`"}}`+"\n\n"),
				upstream.resp.Body,
			))
			svc := &OpenAIGatewayService{httpUpstream: upstream, cfg: rawChatCompletionsTestConfig()}

			_, err := svc.ForwardAsAnthropic(ctx, c, rawGPT56ResponsesOAuthAccount(route.UpstreamModel, route.UpstreamModel), body, "", "")
			require.NoError(t, err)

			require.Equal(t, route.UpstreamModel, gjson.GetBytes(upstream.lastBody, "model").String())
			if stream {
				require.Equal(t, "public-model", anthropicMessageStartModel(t, rec.Body.String()))
			} else {
				require.Equal(t, "public-model", gjson.GetBytes(rec.Body.Bytes(), "model").String())
			}
		})
	}
}

func TestCompositeRouteRestoresPublicModelOnChatCompletionsBridge(t *testing.T) {
	gin.SetMode(gin.TestMode)
	route := publicModelRoute()
	route.TargetPlatform = PlatformOpenAI
	route.UpstreamModel = "gpt-5.6-luna"
	for _, stream := range []bool{false, true} {
		t.Run("stream="+strconv.FormatBool(stream), func(t *testing.T) {
			client := []byte(`{"model":"public-model","max_tokens":16,"messages":[{"role":"user","content":"hello"}],"stream":` + strconv.FormatBool(stream) + `}`)
			ctx, body := routeCompositeBody(t, route, CompositeRouteEndpointMessages, client)

			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(string(body))).WithContext(ctx)
			c.Request.Header.Set("Content-Type", "application/json")
			upstreamBody := `{"id":"chatcmpl_route","object":"chat.completion","model":"gpt-5.6-luna","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}}`
			contentType := "application/json"
			if stream {
				upstreamBody = strings.Join([]string{
					`data: {"id":"chatcmpl_route","object":"chat.completion.chunk","model":"gpt-5.6-luna","choices":[{"index":0,"delta":{"role":"assistant","content":"ok"},"finish_reason":null}]}`,
					"",
					`data: {"id":"chatcmpl_route","object":"chat.completion.chunk","model":"gpt-5.6-luna","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}}`,
					"",
					"data: [DONE]",
					"",
				}, "\n")
				contentType = "text/event-stream"
			}
			upstream := &httpUpstreamRecorder{resp: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{contentType}},
				Body:       io.NopCloser(strings.NewReader(upstreamBody)),
			}}
			account := forceChatMessagesFallbackAccount()
			svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}

			_, err := svc.ForwardAsAnthropic(ctx, c, account, body, "", "")
			require.NoError(t, err)
			require.Equal(t, "gpt-5.6-luna", gjson.GetBytes(upstream.lastBody, "model").String())
			if stream {
				require.Equal(t, "public-model", anthropicMessageStartModel(t, rec.Body.String()))
			} else {
				require.Equal(t, "public-model", gjson.GetBytes(rec.Body.Bytes(), "model").String())
			}
		})
	}
}

func TestRestoreCompositeClientModelOnlyWhenRouteRewroteModel(t *testing.T) {
	body := []byte(`{"id":"msg_1","type":"message","model":"claude-opus-5-5-20260901"}`)
	route := publicModelRoute()

	renamedCtx, _ := routeCompositeBody(t, route, CompositeRouteEndpointMessages, []byte(`{"model":"public-model"}`))
	require.Equal(t, "public-model", gjson.GetBytes(restoreCompositeClientModel(renamedCtx, body), "model").String())
	require.Equal(t, "public-model", compositeClientModel(renamedCtx, "claude-opus-5-5"))

	route.PublicModel = route.UpstreamModel
	sameCtx, _ := routeCompositeBody(t, route, CompositeRouteEndpointMessages, []byte(`{"model":"claude-opus-5-5"}`))
	require.Equal(t, string(body), string(restoreCompositeClientModel(sameCtx, body)))
	require.Equal(t, "claude-opus-5-5", compositeClientModel(sameCtx, "claude-opus-5-5"))

	require.Equal(t, string(body), string(restoreCompositeClientModel(context.Background(), body)))
	require.Equal(t, "claude-opus-5-5", compositeClientModel(context.Background(), "claude-opus-5-5"))

	noModel := []byte(`{"type":"error"}`)
	require.Equal(t, string(noModel), string(restoreCompositeClientModel(renamedCtx, noModel)))
}

func forwardCompositeAnthropicStream(t *testing.T, newAccount func() *Account, upstreamSSE string) string {
	t.Helper()
	gin.SetMode(gin.TestMode)
	client := []byte(`{"model":"public-model","max_tokens":16,"stream":true,"messages":[{"role":"user","content":"hi"}]}`)
	ctx, body := routeCompositeBody(t, publicModelRoute(), CompositeRouteEndpointMessages, client)
	upstream := &anthropicHTTPUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(upstreamSSE)),
	}}
	svc := newForwardPartialUsageServiceForTest(upstream)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil).WithContext(ctx)
	parsed, err := ParseGatewayRequest(NewRequestBodyRef(body), PlatformAnthropic)
	require.NoError(t, err)

	_, err = svc.Forward(ctx, c, newAccount(), parsed)
	require.NoError(t, err)
	return rec.Body.String()
}

func anthropicStreamEventData(t *testing.T, sse, eventType string) string {
	t.Helper()
	for _, line := range strings.Split(sse, "\n") {
		data, ok := strings.CutPrefix(line, "data:")
		data = strings.TrimSpace(data)
		if ok && gjson.Get(data, "type").String() == eventType {
			return data
		}
	}
	t.Fatalf("no %s event in stream:\n%s", eventType, sse)
	return ""
}

func TestCompositeRouteStreamRewritesOnlyMessageStartModel(t *testing.T) {
	accounts := map[string]func() *Account{
		"oauth":              newAnthropicOAuthAccountForPartialUsageTest,
		"apikey-passthrough": newAnthropicAPIKeyAccountForTest,
	}
	for accountName, newAccount := range accounts {
		t.Run(accountName, func(t *testing.T) {
			sse := forwardCompositeAnthropicStream(t, newAccount, strings.Join([]string{
				`event: message_start`,
				`data: {"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","model":"claude-opus-5-5-20260901","content":[],"usage":{"input_tokens":1}}}`,
				"",
				`event: ping`,
				`data: {"type":"ping","message":{"model":"other-model"}}`,
				"",
				`event: message_delta`,
				`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":1}}`,
				"",
				`event: message_stop`,
				`data: {"type":"message_stop"}`,
				"",
				"",
			}, "\n"))

			require.Equal(t, "public-model", gjson.Get(anthropicStreamEventData(t, sse, "message_start"), "message.model").String())
			require.Equal(t, "other-model", gjson.Get(anthropicStreamEventData(t, sse, "ping"), "message.model").String())
		})
	}
}

func TestCompositeRoutePassthroughStreamKeepsMessageStartWithoutModel(t *testing.T) {
	sse := forwardCompositeAnthropicStream(t, newAnthropicAPIKeyAccountForTest, strings.Join([]string{
		`event: message_start`,
		`data:{"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","content":[],"usage":{"input_tokens":1}}}`,
		"",
		`event: message_stop`,
		`data: {"type":"message_stop"}`,
		"",
		"",
	}, "\n"))

	require.False(t, gjson.Get(anthropicStreamEventData(t, sse, "message_start"), "message.model").Exists())
}
