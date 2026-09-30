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

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai_compat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func sessionAffinityTestContext(t *testing.T, path string, body []byte, headers map[string]string) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	for name, value := range headers {
		c.Request.Header.Set(name, value)
	}
	return c
}

func sessionAffinityTestAccount(enabled bool) *Account {
	return &Account{
		ID:          301,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":                              "configured-upstream-key",
			"base_url":                             "http://aisix.example/v1",
			openAISessionAffinityEnabledCredential: enabled,
		},
	}
}

func TestResolveOpenAISessionAffinityID(t *testing.T) {
	valid255Bytes := strings.Repeat("好", 85)
	invalid256Bytes := strings.Repeat("a", 256)
	invalidUTF8 := string([]byte{'s', 'i', 'd', 0xff})

	tests := []struct {
		name            string
		headers         map[string]string
		body            []byte
		includeMessages bool
		want            string
	}{
		{
			name: "header precedence preserves explicit session over cache key",
			headers: map[string]string{
				"session-id":      "stable-session",
				"conversation_id": "lower-priority",
			},
			body: []byte(`{"prompt_cache_key":"separate-cache-key"}`),
			want: "stable-session",
		},
		{
			name: "invalid higher priority candidate falls through",
			headers: map[string]string{
				"session-id":      "bad\tvalue",
				"conversation_id": "valid-conversation",
			},
			want: "valid-conversation",
		},
		{
			name: "body cache key remains supported fallback",
			body: []byte(`{"prompt_cache_key":"body-session"}`),
			want: "body-session",
		},
		{
			name:            "messages Claude header follows existing generic precedence",
			headers:         map[string]string{claudeCodeSessionHeader: "claude-session"},
			body:            []byte(`{"prompt_cache_key":"body-session"}`),
			includeMessages: true,
			want:            "body-session",
		},
		{
			name:            "messages metadata supplies Claude session",
			body:            []byte(`{"metadata":{"user_id":"{\"session_id\":\"metadata-session\"}"}}`),
			includeMessages: true,
			want:            "metadata-session",
		},
		{
			name:    "valid UTF-8 at 255 byte boundary",
			headers: map[string]string{"session_id": valid255Bytes},
			want:    valid255Bytes,
		},
		{
			name:    "over 255 bytes is absent",
			headers: map[string]string{"session_id": invalid256Bytes},
		},
		{
			name:    "invalid UTF-8 is absent",
			headers: map[string]string{"session_id": invalidUTF8},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := sessionAffinityTestContext(t, "/v1/responses", tt.body, tt.headers)
			require.Equal(t, tt.want, resolveOpenAISessionAffinityID(c, tt.body, tt.includeMessages))
		})
	}
}

func TestOpenAISessionAffinityHeaderCrossesEveryHTTPBuilder(t *testing.T) {
	body := []byte(`{"model":"logical-model","input":"hello","prompt_cache_key":"separate-cache-key"}`)
	account := sessionAffinityTestAccount(true)
	svc := &OpenAIGatewayService{cfg: &config.Config{Security: config.SecurityConfig{
		URLAllowlist: config.URLAllowlistConfig{Enabled: false, AllowInsecureHTTP: true},
	}}}

	builders := []struct {
		name  string
		build func(*gin.Context) (*http.Request, error)
	}{
		{
			name: "responses conversion",
			build: func(c *gin.Context) (*http.Request, error) {
				rememberOpenAISessionAffinityID(c, body, false)
				return svc.buildUpstreamRequest(context.Background(), c, account, body, "configured-upstream-key", true, "separate-cache-key", false)
			},
		},
		{
			name: "responses passthrough and websocket HTTP bridge",
			build: func(c *gin.Context) (*http.Request, error) {
				rememberOpenAISessionAffinityID(c, body, false)
				return svc.buildUpstreamRequestOpenAIPassthrough(context.Background(), c, account, body, "configured-upstream-key")
			},
		},
	}

	for _, tt := range builders {
		t.Run(tt.name, func(t *testing.T) {
			c := sessionAffinityTestContext(t, "/v1/responses", body, map[string]string{openCodeSessionIDHeader: "stable-session"})
			req, err := tt.build(c)
			require.NoError(t, err)
			require.Equal(t, "stable-session", req.Header.Get("session_id"))
			require.Equal(t, "Bearer configured-upstream-key", req.Header.Get("Authorization"))
			require.Equal(t, "separate-cache-key", gjson.GetBytes(body, "prompt_cache_key").String())
		})
	}
}

func TestOpenAISessionAffinityHeaderCrossesRawChatBuilder(t *testing.T) {
	body := []byte(`{"model":"logical-model","messages":[{"role":"user","content":"hello"}]}`)
	c := sessionAffinityTestContext(t, "/v1/chat/completions", body, map[string]string{openCodeSessionIDHeader: "stable-session"})
	rememberOpenAISessionAffinityID(c, body, false)

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{}`)),
	}}
	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
	resp, err := svc.sendCCUpstreamRequest(
		context.Background(), c, sessionAffinityTestAccount(true),
		"http://aisix.example/v1/chat/completions", body, false,
		"configured-upstream-key", "", "",
	)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, "stable-session", upstream.lastReq.Header.Get("session_id"))
	require.Equal(t, "Bearer configured-upstream-key", upstream.lastReq.Header.Get("Authorization"))
}

func TestOpenAISessionAffinityHeaderCrossesClaudeCodeMessagesInput(t *testing.T) {
	body := []byte(`{"model":"logical-model","max_tokens":16,"messages":[{"role":"user","content":"hello"}],"stream":false}`)
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"id":"chatcmpl_1","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)),
	}}
	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
	account := sessionAffinityTestAccount(true)
	account.Extra = map[string]any{openai_compat.ExtraKeyResponsesMode: string(openai_compat.ResponsesSupportModeForceChatCompletions)}

	c := sessionAffinityTestContext(t, "/v1/messages", body, map[string]string{claudeCodeSessionHeader: "claude-code-session"})
	_, err := svc.ForwardAsAnthropic(context.Background(), c, account, body, "", "")
	require.NoError(t, err)
	require.Equal(t, "claude-code-session", upstream.lastReq.Header.Get("session_id"))
}

func TestOpenAISessionAffinityHeaderCrossesPublicConversationalPaths(t *testing.T) {
	responseSSE := func(id string) *http.Response {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body: io.NopCloser(strings.NewReader(
				`data: {"type":"response.completed","response":{"id":"` + id + `","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":1}}}` + "\n\n",
			)),
		}
	}
	chatJSON := func() *http.Response {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"id":"chatcmpl_1","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)),
		}
	}

	tests := []struct {
		name    string
		path    string
		body    []byte
		account func() *Account
		resp    func() *http.Response
		forward func(*OpenAIGatewayService, *gin.Context, *Account, []byte) error
	}{
		{
			name: "chat to responses",
			path: "/v1/chat/completions",
			body: []byte(`{"model":"logical-model","messages":[{"role":"user","content":"hello"}],"stream":false}`),
			account: func() *Account {
				a := sessionAffinityTestAccount(true)
				a.Extra = map[string]any{openai_compat.ExtraKeyResponsesMode: string(openai_compat.ResponsesSupportModeForceResponses)}
				return a
			},
			resp: func() *http.Response { return responseSSE("resp_chat") },
			forward: func(s *OpenAIGatewayService, c *gin.Context, a *Account, body []byte) error {
				_, err := s.ForwardAsChatCompletions(context.Background(), c, a, body, "separate-cache-key", "")
				return err
			},
		},
		{
			name: "responses passthrough",
			path: "/v1/responses",
			body: []byte(`{"model":"logical-model","input":"hello","stream":true,"prompt_cache_key":"separate-cache-key"}`),
			account: func() *Account {
				a := sessionAffinityTestAccount(true)
				a.Extra = map[string]any{"openai_passthrough": true}
				return a
			},
			resp: func() *http.Response { return responseSSE("resp_passthrough") },
			forward: func(s *OpenAIGatewayService, c *gin.Context, a *Account, body []byte) error {
				_, err := s.Forward(context.Background(), c, a, body)
				return err
			},
		},
		{
			name:    "messages to responses",
			path:    "/v1/messages",
			body:    []byte(`{"model":"logical-model","max_tokens":16,"messages":[{"role":"user","content":"hello"}],"stream":false}`),
			account: func() *Account { return sessionAffinityTestAccount(true) },
			resp:    func() *http.Response { return responseSSE("resp_messages") },
			forward: func(s *OpenAIGatewayService, c *gin.Context, a *Account, body []byte) error {
				_, err := s.ForwardAsAnthropic(context.Background(), c, a, body, "separate-cache-key", "")
				return err
			},
		},
		{
			name: "messages to raw chat",
			path: "/v1/messages",
			body: []byte(`{"model":"logical-model","max_tokens":16,"messages":[{"role":"user","content":"hello"}],"stream":false}`),
			account: func() *Account {
				a := sessionAffinityTestAccount(true)
				a.Extra = map[string]any{openai_compat.ExtraKeyResponsesMode: string(openai_compat.ResponsesSupportModeForceChatCompletions)}
				return a
			},
			resp: chatJSON,
			forward: func(s *OpenAIGatewayService, c *gin.Context, a *Account, body []byte) error {
				_, err := s.ForwardAsAnthropic(context.Background(), c, a, body, "", "")
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			upstream := &httpUpstreamRecorder{resp: tt.resp()}
			svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
			c := sessionAffinityTestContext(t, tt.path, tt.body, map[string]string{openCodeSessionIDHeader: "stable-session"})
			require.NoError(t, tt.forward(svc, c, tt.account(), tt.body))
			require.Equal(t, "stable-session", upstream.lastReq.Header.Get("session_id"))
			require.Equal(t, "Bearer configured-upstream-key", upstream.lastReq.Header.Get("Authorization"))
		})
	}
}

func TestOpenAISessionAffinityHeaderCrossesWebSocketHTTPBridge(t *testing.T) {
	payload := []byte(`{"type":"response.create","model":"logical-model","input":"hello","stream":true,"prompt_cache_key":"separate-cache-key"}`)
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body: io.NopCloser(strings.NewReader(
			`data: {"type":"response.completed","response":{"id":"resp_ws","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":1}}}` + "\n\n",
		)),
	}}
	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
	c := sessionAffinityTestContext(t, "/v1/responses", nil, map[string]string{openCodeSessionIDHeader: "stable-session"})
	result, err := svc.proxyOpenAIWSHTTPBridgeTurn(
		context.Background(), c, sessionAffinityTestAccount(true), "configured-upstream-key",
		payload, len(payload), "logical-model", "", "", "", "", 1, func([]byte) error { return nil },
	)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "stable-session", upstream.lastReq.Header.Get("session_id"))
}

func TestOpenAISessionAffinityIdentityIsIndependentOfSelectedAccount(t *testing.T) {
	body := []byte(`{"model":"logical-model","input":"hello"}`)
	for _, account := range []*Account{
		{ID: 301, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{openAISessionAffinityEnabledCredential: true}},
		{ID: 909, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{openAISessionAffinityEnabledCredential: true}},
	} {
		c := sessionAffinityTestContext(t, "/v1/responses", body, map[string]string{openCodeSessionIDHeader: "client-supplied-session"})
		rememberOpenAISessionAffinityID(c, body, false)
		req, err := (&OpenAIGatewayService{cfg: &config.Config{Security: config.SecurityConfig{
			URLAllowlist: config.URLAllowlistConfig{Enabled: false, AllowInsecureHTTP: true},
		}}}).buildUpstreamRequestOpenAIPassthrough(context.Background(), c, account, body, "configured-upstream-key")
		require.NoError(t, err)
		require.Equal(t, "client-supplied-session", req.Header.Get("session_id"))
		require.NotContains(t, req.Header.Get("session_id"), "301")
		require.NotContains(t, req.Header.Get("session_id"), "909")
	}
}

func TestOpenAISessionAffinityHeaderRequiresOptInAPIKeyAccount(t *testing.T) {
	body := []byte(`{"model":"logical-model","input":"hello"}`)
	svc := &OpenAIGatewayService{cfg: &config.Config{Security: config.SecurityConfig{
		URLAllowlist: config.URLAllowlistConfig{Enabled: false, AllowInsecureHTTP: true},
	}}}

	for _, account := range []*Account{
		sessionAffinityTestAccount(false),
		{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{openAISessionAffinityEnabledCredential: true}},
		{Platform: PlatformGrok, Type: AccountTypeAPIKey, Credentials: map[string]any{openAISessionAffinityEnabledCredential: true}},
	} {
		c := sessionAffinityTestContext(t, "/v1/responses", body, map[string]string{openCodeSessionIDHeader: "private-session"})
		rememberOpenAISessionAffinityID(c, body, false)
		req, err := svc.buildUpstreamRequestOpenAIPassthrough(context.Background(), c, account, body, "configured-upstream-key")
		require.NoError(t, err)
		require.Empty(t, req.Header.Get("session_id"))
	}
}
