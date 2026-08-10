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

	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai_compat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func openAIFastHTTPPolicy(action string) *OpenAIFastPolicySettings {
	if action == BetaPolicyActionPass {
		return DefaultOpenAIFastPolicySettings()
	}
	return &OpenAIFastPolicySettings{Rules: []OpenAIFastPolicyRule{{
		ServiceTier:  OpenAIFastTierAny,
		Action:       action,
		Scope:        BetaPolicyScopeAll,
		ErrorMessage: "fast policy blocked",
	}}}
}

func openAIFastHTTPTestService(t *testing.T, action string, upstream *httpUpstreamRecorder) *OpenAIGatewayService {
	t.Helper()
	svc := newOpenAIGatewayServiceWithSettings(t, openAIFastHTTPPolicy(action))
	svc.cfg = rawChatCompletionsTestConfig()
	svc.httpUpstream = upstream
	return svc
}

func openAIFastHTTPChatResponse(stream bool) *http.Response {
	if stream {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body: io.NopCloser(strings.NewReader(strings.Join([]string{
				`data: {"id":"chatcmpl_fast","object":"chat.completion.chunk","model":"gpt-5.6-sol","choices":[{"index":0,"delta":{"content":"ok"},"finish_reason":"stop"}]}`,
				"",
				`data: {"id":"chatcmpl_fast","object":"chat.completion.chunk","model":"gpt-5.6-sol","choices":[],"usage":{"prompt_tokens":2,"completion_tokens":1,"total_tokens":3}}`,
				"",
				"data: [DONE]",
				"",
			}, "\n"))),
		}
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body: io.NopCloser(strings.NewReader(
			`{"id":"chatcmpl_fast","object":"chat.completion","model":"gpt-5.6-sol","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":1,"total_tokens":3}}`,
		)),
	}
}

func openAIFastHTTPResponsesSSE() *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body: io.NopCloser(strings.NewReader(
			`data: {"type":"response.completed","response":{"id":"resp_fast","object":"response","model":"gpt-5.6-sol","status":"completed","output":[{"type":"message","id":"msg_fast","role":"assistant","status":"completed","content":[{"type":"output_text","text":"ok"}]}],"usage":{"input_tokens":2,"output_tokens":1,"total_tokens":3}}}` + "\n\n",
		)),
	}
}

func requireOpenAIFastHTTPServiceTier(t *testing.T, tier *string, want string) {
	t.Helper()
	if want == "" {
		require.Nil(t, tier)
		return
	}
	require.NotNil(t, tier)
	require.Equal(t, want, *tier)
}

func TestRawChatFastPolicyUsesFinalOutboundTier(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name         string
		action       string
		inputTier    string
		stream       bool
		wantTier     string
		wantBlocked  bool
		wantUpstream bool
	}{
		{name: "pass normalizes fast", action: BetaPolicyActionPass, inputTier: "fast", wantTier: "priority", wantUpstream: true},
		{name: "filter clears streaming metadata", action: BetaPolicyActionFilter, inputTier: "priority", stream: true, wantUpstream: true},
		{name: "force priority updates metadata", action: OpenAIFastPolicyActionForcePriority, inputTier: "flex", wantTier: "priority", wantUpstream: true},
		{name: "block skips upstream", action: BetaPolicyActionBlock, inputTier: "priority", wantBlocked: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := []byte(`{"model":"gpt-5.6-sol","messages":[{"role":"user","content":"hi"}],"stream":` + boolJSON(tt.stream) + `,"service_tier":"` + tt.inputTier + `"}`)
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
			upstream := &httpUpstreamRecorder{resp: openAIFastHTTPChatResponse(tt.stream)}
			svc := openAIFastHTTPTestService(t, tt.action, upstream)

			result, err := svc.forwardAsRawChatCompletions(context.Background(), c, rawChatCompletionsTestAccount(), body, "")
			if tt.wantBlocked {
				var blocked *OpenAIFastBlockedError
				require.ErrorAs(t, err, &blocked)
				require.Nil(t, result)
				require.Equal(t, http.StatusForbidden, recorder.Code)
				require.Empty(t, upstream.requests)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, result)
			require.Equal(t, tt.wantUpstream, len(upstream.requests) == 1)
			require.Equal(t, tt.wantTier, gjson.GetBytes(upstream.lastBody, "service_tier").String())
			requireOpenAIFastHTTPServiceTier(t, result.ServiceTier, tt.wantTier)
		})
	}
}

func TestResponsesRawChatFallbackFastPolicyUsesFinalOutboundTier(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name      string
		action    string
		inputTier string
		stream    bool
		wantTier  string
	}{
		{name: "filter nonstreaming", action: BetaPolicyActionFilter, inputTier: "priority"},
		{name: "force priority streaming", action: OpenAIFastPolicyActionForcePriority, inputTier: "flex", stream: true, wantTier: "priority"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := []byte(`{"model":"gpt-5.6-sol","input":"hi","stream":` + boolJSON(tt.stream) + `,"service_tier":"` + tt.inputTier + `"}`)
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
			upstream := &httpUpstreamRecorder{resp: openAIFastHTTPChatResponse(tt.stream)}
			svc := openAIFastHTTPTestService(t, tt.action, upstream)

			result, err := svc.Forward(context.Background(), c, forceChatResponsesFallbackAccount(), body)
			require.NoError(t, err)
			require.NotNil(t, result)
			require.Equal(t, tt.wantTier, gjson.GetBytes(upstream.lastBody, "service_tier").String())
			requireOpenAIFastHTTPServiceTier(t, result.ServiceTier, tt.wantTier)
		})
	}
}

func TestChatResponsesFastPolicyUsesFinalOutboundTier(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name      string
		action    string
		inputTier string
		stream    bool
		wantTier  string
	}{
		{name: "pass normalizes fast nonstreaming", action: BetaPolicyActionPass, inputTier: "fast", wantTier: "priority"},
		{name: "filter clears streaming metadata", action: BetaPolicyActionFilter, inputTier: "priority", stream: true},
		{name: "force priority updates metadata", action: OpenAIFastPolicyActionForcePriority, inputTier: "flex", wantTier: "priority"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := []byte(`{"model":"gpt-5.6-sol","messages":[{"role":"user","content":"hi"}],"stream":` + boolJSON(tt.stream) + `,"service_tier":"` + tt.inputTier + `"}`)
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
			upstream := &httpUpstreamRecorder{resp: openAIFastHTTPResponsesSSE()}
			svc := openAIFastHTTPTestService(t, tt.action, upstream)
			account := rawChatCompletionsTestAccount()
			account.Extra = map[string]any{
				openai_compat.ExtraKeyResponsesMode:      string(openai_compat.ResponsesSupportModeAuto),
				openai_compat.ExtraKeyResponsesSupported: true,
			}

			result, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")
			require.NoError(t, err)
			require.NotNil(t, result)
			require.Equal(t, tt.wantTier, gjson.GetBytes(upstream.lastBody, "service_tier").String())
			requireOpenAIFastHTTPServiceTier(t, result.ServiceTier, tt.wantTier)
		})
	}
}

func TestMessagesResponsesFastPolicyUsesFinalOutboundTier(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name     string
		action   string
		stream   bool
		wantTier string
	}{
		{name: "pass beta fast", action: BetaPolicyActionPass, stream: true, wantTier: "priority"},
		{name: "filter clears nonstreaming metadata", action: BetaPolicyActionFilter},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := []byte(`{"model":"gpt-5.6-sol","max_tokens":16,"messages":[{"role":"user","content":"hi"}],"stream":` + boolJSON(tt.stream) + `}`)
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
			c.Request.Header.Set("anthropic-beta", claude.BetaFastMode)
			upstream := &httpUpstreamRecorder{resp: openAIFastHTTPResponsesSSE()}
			svc := openAIFastHTTPTestService(t, tt.action, upstream)
			account := rawChatCompletionsTestAccount()
			account.Extra = map[string]any{
				openai_compat.ExtraKeyResponsesMode:      string(openai_compat.ResponsesSupportModeAuto),
				openai_compat.ExtraKeyResponsesSupported: true,
			}

			result, err := svc.ForwardAsAnthropic(context.Background(), c, account, body, "", "")
			require.NoError(t, err)
			require.NotNil(t, result)
			require.Equal(t, tt.wantTier, gjson.GetBytes(upstream.lastBody, "service_tier").String())
			requireOpenAIFastHTTPServiceTier(t, result.ServiceTier, tt.wantTier)
		})
	}
}

func TestMessagesRawChatFallbackFastPolicy(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name        string
		action      string
		stream      bool
		wantTier    string
		wantBlocked bool
	}{
		{name: "beta fast becomes priority", action: BetaPolicyActionPass, wantTier: "priority"},
		{name: "filter clears streaming metadata", action: BetaPolicyActionFilter, stream: true},
		{name: "block is anthropic 403 with no upstream", action: BetaPolicyActionBlock, wantBlocked: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := []byte(`{"model":"gpt-5.6-sol","max_tokens":16,"messages":[{"role":"user","content":"hi"}],"stream":` + boolJSON(tt.stream) + `}`)
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
			c.Request.Header.Set("anthropic-beta", claude.BetaFastMode)
			upstream := &httpUpstreamRecorder{resp: openAIFastHTTPChatResponse(tt.stream)}
			svc := openAIFastHTTPTestService(t, tt.action, upstream)

			result, err := svc.ForwardAsAnthropic(context.Background(), c, forceChatMessagesFallbackAccount(), body, "", "")
			if tt.wantBlocked {
				var blocked *OpenAIFastBlockedError
				require.ErrorAs(t, err, &blocked)
				require.Nil(t, result)
				require.Equal(t, http.StatusForbidden, recorder.Code)
				require.Equal(t, "error", gjson.Get(recorder.Body.String(), "type").String())
				require.Equal(t, "forbidden_error", gjson.Get(recorder.Body.String(), "error.type").String())
				require.Empty(t, upstream.requests)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, result)
			require.Equal(t, tt.wantTier, gjson.GetBytes(upstream.lastBody, "service_tier").String())
			requireOpenAIFastHTTPServiceTier(t, result.ServiceTier, tt.wantTier)
		})
	}
}

func boolJSON(value bool) string {
	if value {
		return "true"
	}
	return "false"
}
