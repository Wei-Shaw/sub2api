package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type openAICompatFailingWriter struct {
	gin.ResponseWriter
	failAfter int
	writes    int
}

func (w *openAICompatFailingWriter) Write(p []byte) (int, error) {
	if w.writes >= w.failAfter {
		return 0, errors.New("write failed: client disconnected")
	}
	w.writes++
	return w.ResponseWriter.Write(p)
}

type openAICompatBlockingReadCloser struct {
	data      []byte
	offset    int
	closed    chan struct{}
	closeOnce sync.Once
}

func newOpenAICompatBlockingReadCloser(data []byte) *openAICompatBlockingReadCloser {
	return &openAICompatBlockingReadCloser{
		data:   data,
		closed: make(chan struct{}),
	}
}

func (r *openAICompatBlockingReadCloser) Read(p []byte) (int, error) {
	if r.offset < len(r.data) {
		n := copy(p, r.data[r.offset:])
		r.offset += n
		return n, nil
	}
	<-r.closed
	return 0, io.EOF
}

func (r *openAICompatBlockingReadCloser) Close() error {
	r.closeOnce.Do(func() {
		close(r.closed)
	})
	return nil
}

func TestNormalizeOpenAICompatRequestedModel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "gpt reasoning alias strips xhigh", input: "gpt-5.4-xhigh", want: "gpt-5.4"},
		{name: "gpt reasoning alias strips none", input: "gpt-5.4-none", want: "gpt-5.4"},
		{name: "codex max model stays intact", input: "gpt-5.1-codex-max", want: "gpt-5.1-codex-max"},
		{name: "non openai model unchanged", input: "claude-opus-4-6", want: "claude-opus-4-6"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, NormalizeOpenAICompatRequestedModel(tt.input))
		})
	}
}

func TestApplyOpenAICompatModelNormalization(t *testing.T) {
	t.Parallel()

	t.Run("derives xhigh from model suffix when output config missing", func(t *testing.T) {
		req := &apicompat.AnthropicRequest{Model: "gpt-5.4-xhigh"}

		applyOpenAICompatModelNormalization(req)

		require.Equal(t, "gpt-5.4", req.Model)
		require.NotNil(t, req.OutputConfig)
		require.Equal(t, "max", req.OutputConfig.Effort)
	})

	t.Run("explicit output config wins over model suffix", func(t *testing.T) {
		req := &apicompat.AnthropicRequest{
			Model:        "gpt-5.4-xhigh",
			OutputConfig: &apicompat.AnthropicOutputConfig{Effort: "low"},
		}

		applyOpenAICompatModelNormalization(req)

		require.Equal(t, "gpt-5.4", req.Model)
		require.NotNil(t, req.OutputConfig)
		require.Equal(t, "low", req.OutputConfig.Effort)
	})

	t.Run("non openai model is untouched", func(t *testing.T) {
		req := &apicompat.AnthropicRequest{Model: "claude-opus-4-6"}

		applyOpenAICompatModelNormalization(req)

		require.Equal(t, "claude-opus-4-6", req.Model)
		require.Nil(t, req.OutputConfig)
	})
}

func TestForwardAsAnthropic_NormalizesRoutingAndEffortForGpt54XHigh(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"gpt-5.4-xhigh","max_tokens":16,"messages":[{"role":"user","content":"hello"}],"stream":false}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstreamBody := strings.Join([]string{
		`data: {"type":"response.completed","response":{"id":"resp_1","object":"response","model":"gpt-5.4","status":"completed","output":[{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"ok"}]}],"usage":{"input_tokens":5,"output_tokens":2,"total_tokens":7}}}`,
		"",
		"data: [DONE]",
		"",
	}, "\n")
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "x-request-id": []string{"rid_compat"}},
		Body:       io.NopCloser(strings.NewReader(upstreamBody)),
	}}

	svc := &OpenAIGatewayService{
		httpUpstream: upstream,
		cfg:          &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: false}}},
	}
	account := &Account{
		ID:          1,
		Name:        "openai-oauth",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token":       "oauth-token",
			"chatgpt_account_id": "chatgpt-acc",
			"model_mapping": map[string]any{
				"gpt-5.4": "gpt-5.4",
			},
		},
	}

	result, err := svc.ForwardAsAnthropic(context.Background(), c, account, body, "", "gpt-5.1")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "gpt-5.4-xhigh", result.Model)
	require.Equal(t, "gpt-5.4", result.UpstreamModel)
	require.Equal(t, "gpt-5.4", result.BillingModel)
	require.NotNil(t, result.ReasoningEffort)
	require.Equal(t, "xhigh", *result.ReasoningEffort)

	require.Equal(t, "gpt-5.4", gjson.GetBytes(upstream.lastBody, "model").String())
	require.Equal(t, "xhigh", gjson.GetBytes(upstream.lastBody, "reasoning.effort").String())
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "gpt-5.4-xhigh", gjson.GetBytes(rec.Body.Bytes(), "model").String())
	require.Equal(t, "ok", gjson.GetBytes(rec.Body.Bytes(), "content.0.text").String())
	t.Logf("upstream body: %s", string(upstream.lastBody))
	t.Logf("response body: %s", rec.Body.String())
}

func TestForwardAsAnthropic_InjectsPromptCacheKeyForAPIKeyMessagesDispatch(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"claude-sonnet-4-5","max_tokens":16,"metadata":{"user_id":"claude-session-1"},"messages":[{"role":"user","content":"hello"}],"stream":false}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstreamBody := strings.Join([]string{
		`data: {"type":"response.completed","response":{"id":"resp_1","object":"response","model":"gpt-5.3-codex","status":"completed","output":[{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"ok"}]}],"usage":{"input_tokens":5,"output_tokens":2,"total_tokens":7,"input_tokens_details":{"cached_tokens":3}}}}`,
		"",
		"data: [DONE]",
		"",
	}, "\n")
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "x-request-id": []string{"rid_cache_key"}},
		Body:       io.NopCloser(strings.NewReader(upstreamBody)),
	}}

	svc := &OpenAIGatewayService{
		httpUpstream: upstream,
		cfg:          &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: false}}},
	}
	account := &Account{
		ID:          1,
		Name:        "openai-apikey",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": "https://api.openai.com/v1",
		},
	}

	result, err := svc.ForwardAsAnthropic(context.Background(), c, account, body, "stable-cache-key", "gpt-5.3-codex")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "stable-cache-key", gjson.GetBytes(upstream.lastBody, "prompt_cache_key").String())
	require.Equal(t, "gpt-5.3-codex", gjson.GetBytes(upstream.lastBody, "model").String())
	require.Equal(t, 3, result.Usage.CacheReadInputTokens)
}

func TestForwardAsAnthropic_AutoDerivesPromptCacheKeyWhenMessagesDispatchHasNoSessionID(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"claude-sonnet-4-5","max_tokens":16,"system":"You are helpful.","messages":[{"role":"user","content":"open repo"}],"stream":false}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstreamBody := strings.Join([]string{
		`data: {"type":"response.completed","response":{"id":"resp_1","object":"response","model":"gpt-5.3-codex","status":"completed","output":[{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"ok"}]}],"usage":{"input_tokens":5,"output_tokens":2,"total_tokens":7,"input_tokens_details":{"cached_tokens":3}}}}`,
		"",
		"data: [DONE]",
		"",
	}, "\n")
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "x-request-id": []string{"rid_auto_cache_key"}},
		Body:       io.NopCloser(strings.NewReader(upstreamBody)),
	}}

	svc := &OpenAIGatewayService{
		httpUpstream: upstream,
		cfg:          &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: false}}},
	}
	account := &Account{
		ID:          1,
		Name:        "openai-apikey",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": "https://api.openai.com/v1",
		},
	}

	result, err := svc.ForwardAsAnthropic(context.Background(), c, account, body, "", "gpt-5.3-codex")
	require.NoError(t, err)
	require.NotNil(t, result)
	cacheKey := gjson.GetBytes(upstream.lastBody, "prompt_cache_key").String()
	require.NotEmpty(t, cacheKey)
	require.True(t, strings.HasPrefix(cacheKey, "anthropic-digest-"))
	require.Equal(t, generateSessionUUID(isolateOpenAISessionID(0, cacheKey)), upstream.lastReq.Header.Get("session_id"))
}

func TestForwardAsAnthropic_DoesNotAutoDerivePromptCacheKeyForNonCodexModel(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"claude-sonnet-4-5","max_tokens":16,"messages":[{"role":"user","content":"hello"}],"stream":false}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstreamBody := strings.Join([]string{
		`data: {"type":"response.completed","response":{"id":"resp_1","object":"response","model":"gpt-4o","status":"completed","output":[{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"ok"}]}],"usage":{"input_tokens":5,"output_tokens":2,"total_tokens":7}}}`,
		"",
		"data: [DONE]",
		"",
	}, "\n")
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "x-request-id": []string{"rid_no_cache_key"}},
		Body:       io.NopCloser(strings.NewReader(upstreamBody)),
	}}

	svc := &OpenAIGatewayService{
		httpUpstream: upstream,
		cfg:          &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: false}}},
	}
	account := &Account{
		ID:          1,
		Name:        "openai-apikey",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": "https://api.openai.com/v1",
		},
	}

	result, err := svc.ForwardAsAnthropic(context.Background(), c, account, body, "", "gpt-4o")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, gjson.GetBytes(upstream.lastBody, "prompt_cache_key").Exists())
	require.Empty(t, upstream.lastReq.Header.Get("session_id"))
}

func TestForwardAsAnthropic_TrimsFullReplayOnlyForCodexCompatModels(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	messages := make([]string, 0, openAICompatAnthropicReplayMaxTailMessages+3)
	for i := 0; i < openAICompatAnthropicReplayMaxTailMessages+3; i++ {
		messages = append(messages, `{"role":"user","content":"message-`+fmt.Sprintf("%02d", i)+`"}`)
	}
	body := []byte(`{"model":"claude-sonnet-4-5","max_tokens":16,"messages":[` + strings.Join(messages, ",") + `],"stream":false}`)

	run := func(t *testing.T, mappedModel string) []byte {
		t.Helper()

		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")

		upstreamBody := strings.Join([]string{
			`data: {"type":"response.completed","response":{"id":"resp_1","object":"response","model":"` + mappedModel + `","status":"completed","output":[{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"ok"}]}],"usage":{"input_tokens":5,"output_tokens":2,"total_tokens":7}}}`,
			"",
			"data: [DONE]",
			"",
		}, "\n")
		upstream := &httpUpstreamRecorder{resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "x-request-id": []string{"rid_trim"}},
			Body:       io.NopCloser(strings.NewReader(upstreamBody)),
		}}

		svc := &OpenAIGatewayService{
			httpUpstream: upstream,
			cfg:          &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: false}}},
		}
		account := &Account{
			ID:          1,
			Name:        "openai-apikey",
			Platform:    PlatformOpenAI,
			Type:        AccountTypeAPIKey,
			Concurrency: 1,
			Credentials: map[string]any{
				"api_key":  "sk-test",
				"base_url": "https://api.openai.com/v1",
			},
		}

		result, err := svc.ForwardAsAnthropic(context.Background(), c, account, body, "", mappedModel)
		require.NoError(t, err)
		require.NotNil(t, result)
		return upstream.lastBody
	}

	codexBody := run(t, "gpt-5.3-codex")
	require.Equal(t, int64(openAICompatAnthropicReplayMaxTailMessages+1), gjson.GetBytes(codexBody, "input.#").Int())
	require.Equal(t, "developer", gjson.GetBytes(codexBody, "input.0.role").String())
	require.Contains(t, gjson.GetBytes(codexBody, "input.0.content.0.text").String(), "<sub2api-claude-code-todo-guard>")
	require.Equal(t, "message-03", gjson.GetBytes(codexBody, "input.1.content.0.text").String())
	require.Equal(t, "message-14", gjson.GetBytes(codexBody, "input.12.content.0.text").String())

	nonCompatBody := run(t, "gpt-4o")
	require.Equal(t, int64(openAICompatAnthropicReplayMaxTailMessages+3), gjson.GetBytes(nonCompatBody, "input.#").Int())
	require.Equal(t, "message-00", gjson.GetBytes(nonCompatBody, "input.0.content.0.text").String())
}

func TestForwardAsAnthropic_OAuthCompatKeepsFullReplayForCacheGrowth(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	messages := make([]string, 0, openAICompatAnthropicReplayMaxTailMessages+3)
	for i := 0; i < openAICompatAnthropicReplayMaxTailMessages+3; i++ {
		messages = append(messages, `{"role":"user","content":"message-`+fmt.Sprintf("%02d", i)+`"}`)
	}
	body := []byte(`{"model":"claude-sonnet-4-5","max_tokens":16,"messages":[` + strings.Join(messages, ",") + `],"stream":false}`)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{resp: openAICompatSSECompletedResponse("resp_oauth_trim", "gpt-5.4")}
	svc := &OpenAIGatewayService{
		httpUpstream: upstream,
		cfg:          &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: false}}},
	}
	account := &Account{
		ID:          1,
		Name:        "openai-oauth",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token":       "oauth-token",
			"chatgpt_account_id": "chatgpt-acc",
		},
	}

	result, err := svc.ForwardAsAnthropic(context.Background(), c, account, body, "", "gpt-5.4")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, int64(openAICompatAnthropicReplayMaxTailMessages+4), gjson.GetBytes(upstream.lastBody, "input.#").Int())
	require.Equal(t, "developer", gjson.GetBytes(upstream.lastBody, "input.0.role").String())
	require.Contains(t, gjson.GetBytes(upstream.lastBody, "input.0.content.0.text").String(), "<sub2api-claude-code-todo-guard>")
	require.Equal(t, "message-00", gjson.GetBytes(upstream.lastBody, "input.1.content.0.text").String())
	require.Equal(t, "message-14", gjson.GetBytes(upstream.lastBody, "input.15.content.0.text").String())
	require.False(t, gjson.GetBytes(upstream.lastBody, "prompt_cache_key").Exists())
}

func TestForwardAsAnthropic_AttachesPreviousResponseIDForCompatContinuation(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	upstream := &httpUpstreamRecorder{}
	svc := &OpenAIGatewayService{
		httpUpstream: upstream,
		cfg:          &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: false}}},
	}
	account := &Account{
		ID:          1,
		Name:        "openai-apikey",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": "https://api.openai.com/v1",
		},
	}

	firstBody := []byte(`{"model":"claude-sonnet-4-5","max_tokens":16,"messages":[{"role":"user","content":"first"}],"stream":false}`)
	upstream.resp = openAICompatSSECompletedResponse("resp_first", "gpt-5.3-codex")
	firstRec := httptest.NewRecorder()
	firstCtx, _ := gin.CreateTestContext(firstRec)
	firstCtx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(firstBody))
	firstCtx.Request.Header.Set("Content-Type", "application/json")

	firstResult, err := svc.ForwardAsAnthropic(context.Background(), firstCtx, account, firstBody, "stable-cache-key", "gpt-5.3-codex")
	require.NoError(t, err)
	require.NotNil(t, firstResult)
	require.Equal(t, "resp_first", firstResult.ResponseID)
	require.False(t, gjson.GetBytes(upstream.lastBody, "previous_response_id").Exists())

	secondBody := []byte(`{"model":"claude-sonnet-4-5","max_tokens":16,"messages":[{"role":"user","content":"first"},{"role":"assistant","content":"ok"},{"role":"user","content":"second"}],"stream":false}`)
	upstream.resp = openAICompatSSECompletedResponse("resp_second", "gpt-5.3-codex")
	secondRec := httptest.NewRecorder()
	secondCtx, _ := gin.CreateTestContext(secondRec)
	secondCtx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(secondBody))
	secondCtx.Request.Header.Set("Content-Type", "application/json")

	secondResult, err := svc.ForwardAsAnthropic(context.Background(), secondCtx, account, secondBody, "stable-cache-key", "gpt-5.3-codex")
	require.NoError(t, err)
	require.NotNil(t, secondResult)
	require.Equal(t, "resp_second", secondResult.ResponseID)
	require.Equal(t, "resp_first", gjson.GetBytes(upstream.lastBody, "previous_response_id").String())
	require.Equal(t, int64(2), gjson.GetBytes(upstream.lastBody, "input.#").Int())
	require.Equal(t, "developer", gjson.GetBytes(upstream.lastBody, "input.0.role").String())
	require.Contains(t, gjson.GetBytes(upstream.lastBody, "input.0.content.0.text").String(), "<sub2api-claude-code-todo-guard>")
	require.Equal(t, "second", gjson.GetBytes(upstream.lastBody, "input.1.content.0.text").String())
}

func TestForwardAsAnthropic_ReplaysWithoutContinuationWhenPreviousResponseMissing(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	upstream := &httpUpstreamRecorder{}
	svc := &OpenAIGatewayService{
		httpUpstream: upstream,
		cfg:          &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: false}}},
	}
	account := &Account{
		ID:          1,
		Name:        "openai-apikey",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": "https://api.openai.com/v1",
		},
	}

	svc.bindOpenAICompatSessionResponseID(context.Background(), nil, account, "stable-cache-key", "resp_missing")
	secondBody := []byte(`{"model":"claude-sonnet-4-5","max_tokens":16,"messages":[{"role":"user","content":"first"},{"role":"assistant","content":"ok"},{"role":"user","content":"second"}],"stream":false}`)
	upstream.responses = []*http.Response{
		{
			StatusCode: http.StatusBadRequest,
			Header:     http.Header{"Content-Type": []string{"application/json"}, "x-request-id": []string{"rid_prev_missing"}},
			Body:       io.NopCloser(strings.NewReader(`{"error":{"code":"previous_response_not_found","message":"previous response not found"}}`)),
		},
		openAICompatSSECompletedResponse("resp_replayed", "gpt-5.3-codex"),
	}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(secondBody))
	c.Request.Header.Set("Content-Type", "application/json")

	result, err := svc.ForwardAsAnthropic(context.Background(), c, account, secondBody, "stable-cache-key", "gpt-5.3-codex")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "resp_replayed", result.ResponseID)
	require.Len(t, upstream.requests, 2)
	require.Equal(t, "resp_missing", gjson.GetBytes(upstream.bodies[0], "previous_response_id").String())
	require.False(t, gjson.GetBytes(upstream.bodies[1], "previous_response_id").Exists())
	require.Equal(t, int64(4), gjson.GetBytes(upstream.bodies[1], "input.#").Int())
	require.Equal(t, "developer", gjson.GetBytes(upstream.bodies[1], "input.0.role").String())
	require.Contains(t, gjson.GetBytes(upstream.bodies[1], "input.0.content.0.text").String(), "<sub2api-claude-code-todo-guard>")
	require.Equal(t, "first", gjson.GetBytes(upstream.bodies[1], "input.1.content.0.text").String())
	require.Equal(t, "second", gjson.GetBytes(upstream.bodies[1], "input.3.content.0.text").String())
}

func TestForwardAsAnthropic_DisablesAPIKeyContinuationWhenUpstreamRequiresWebSocketV2(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	upstream := &httpUpstreamRecorder{}
	svc := &OpenAIGatewayService{
		httpUpstream: upstream,
		cfg:          &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: false}}},
	}
	account := &Account{
		ID:          1,
		Name:        "openai-apikey",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": "https://api.openai.com/v1",
		},
	}

	svc.bindOpenAICompatSessionResponseID(context.Background(), nil, account, "stable-cache-key", "resp_http_unsupported")
	body := []byte(`{"model":"claude-sonnet-4-5","max_tokens":16,"messages":[{"role":"user","content":"first"},{"role":"assistant","content":"ok"},{"role":"user","content":"second"}],"stream":false}`)
	upstream.responses = []*http.Response{
		{
			StatusCode: http.StatusBadRequest,
			Header:     http.Header{"Content-Type": []string{"application/json"}, "x-request-id": []string{"rid_prev_http_unsupported"}},
			Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"previous_response_id is only supported on Responses WebSocket v2","type":"invalid_request_error"}}`)),
		},
		openAICompatSSECompletedResponse("resp_replayed", "gpt-5.5"),
		openAICompatSSECompletedResponse("resp_later", "gpt-5.5"),
	}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	result, err := svc.ForwardAsAnthropic(context.Background(), c, account, body, "stable-cache-key", "gpt-5.5")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "resp_replayed", result.ResponseID)
	require.Len(t, upstream.requests, 2)
	require.Equal(t, "resp_http_unsupported", gjson.GetBytes(upstream.bodies[0], "previous_response_id").String())
	require.False(t, gjson.GetBytes(upstream.bodies[1], "previous_response_id").Exists())

	laterRec := httptest.NewRecorder()
	laterCtx, _ := gin.CreateTestContext(laterRec)
	laterCtx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	laterCtx.Request.Header.Set("Content-Type", "application/json")

	laterResult, err := svc.ForwardAsAnthropic(context.Background(), laterCtx, account, body, "stable-cache-key", "gpt-5.5")
	require.NoError(t, err)
	require.NotNil(t, laterResult)
	require.Equal(t, "resp_later", laterResult.ResponseID)
	require.Len(t, upstream.requests, 3)
	require.False(t, gjson.GetBytes(upstream.bodies[2], "previous_response_id").Exists())
}

func TestForwardAsAnthropic_APIKeyMetadataSessionSurvivesChangingCacheControlAnchorAfterContinuationDisabled(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	metadata := `{"user_id":"{\"device_id\":\"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\",\"account_uuid\":\"\",\"session_id\":\"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa\"}"}`
	firstBody := []byte(`{"model":"claude-haiku-4-5-20251001","max_tokens":16,"metadata":` + metadata + `,"system":[{"type":"text","text":"project docs","cache_control":{"type":"ephemeral"}}],"messages":[{"role":"user","content":"first"}],"stream":false}`)
	messages := make([]string, 0, openAICompatAnthropicReplayMaxTailMessages+4)
	messages = append(messages, `{"role":"user","content":[{"type":"text","text":"rewritten context","cache_control":{"type":"ephemeral"}}]}`)
	for i := 1; i < openAICompatAnthropicReplayMaxTailMessages+4; i++ {
		messages = append(messages, `{"role":"user","content":"message-`+fmt.Sprintf("%02d", i)+`"}`)
	}
	secondBody := []byte(`{"model":"claude-haiku-4-5-20251001","max_tokens":16,"metadata":` + metadata + `,"messages":[` + strings.Join(messages, ",") + `],"stream":false}`)

	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		openAICompatSSECompletedResponse("resp_first", "gpt-5.4-mini"),
		openAICompatSSECompletedResponse("resp_second", "gpt-5.4-mini"),
	}}
	svc := &OpenAIGatewayService{
		httpUpstream: upstream,
		cfg:          &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: false}}},
	}
	account := &Account{
		ID:          1,
		Name:        "openai-apikey",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": "https://api.openai.com/v1",
		},
	}

	firstRec := httptest.NewRecorder()
	firstCtx, _ := gin.CreateTestContext(firstRec)
	firstCtx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(firstBody))
	firstCtx.Request.Header.Set("Content-Type", "application/json")

	firstResult, err := svc.ForwardAsAnthropic(context.Background(), firstCtx, account, firstBody, "", "gpt-5.4-mini")
	require.NoError(t, err)
	require.NotNil(t, firstResult)
	firstKey := gjson.GetBytes(upstream.bodies[0], "prompt_cache_key").String()
	require.NotEmpty(t, firstKey)
	require.True(t, strings.HasPrefix(firstKey, "anthropic-metadata-"))

	svc.disableOpenAICompatSessionContinuation(context.Background(), nil, account, firstKey)

	secondRec := httptest.NewRecorder()
	secondCtx, _ := gin.CreateTestContext(secondRec)
	secondCtx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(secondBody))
	secondCtx.Request.Header.Set("Content-Type", "application/json")

	secondResult, err := svc.ForwardAsAnthropic(context.Background(), secondCtx, account, secondBody, "", "gpt-5.4-mini")
	require.NoError(t, err)
	require.NotNil(t, secondResult)
	require.Len(t, upstream.requests, 2)
	require.Equal(t, firstKey, gjson.GetBytes(upstream.bodies[1], "prompt_cache_key").String())
	require.False(t, gjson.GetBytes(upstream.bodies[1], "previous_response_id").Exists())
	require.Equal(t, int64(openAICompatAnthropicReplayMaxTailMessages+5), gjson.GetBytes(upstream.bodies[1], "input.#").Int())
	require.Equal(t, "developer", gjson.GetBytes(upstream.bodies[1], "input.0.role").String())
	require.Contains(t, gjson.GetBytes(upstream.bodies[1], "input.0.content.0.text").String(), "<sub2api-claude-code-todo-guard>")
	require.Equal(t, "rewritten context", gjson.GetBytes(upstream.bodies[1], "input.1.content.0.text").String())
	require.Equal(t, "message-15", gjson.GetBytes(upstream.bodies[1], "input.16.content.0.text").String())
}

func TestForwardAsAnthropic_DoesNotAttachPreviousResponseIDForOAuthCompat(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	upstream := &httpUpstreamRecorder{resp: openAICompatSSECompletedResponse("resp_oauth_next", "gpt-5.4")}
	svc := &OpenAIGatewayService{
		httpUpstream: upstream,
		cfg:          &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: false}}},
	}
	account := &Account{
		ID:          1,
		Name:        "openai-oauth",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token":       "oauth-token",
			"chatgpt_account_id": "chatgpt-acc",
		},
	}
	svc.bindOpenAICompatSessionResponseID(context.Background(), nil, account, "stable-cache-key", "resp_oauth_prev")

	body := []byte(`{"model":"claude-sonnet-4-5","max_tokens":16,"messages":[{"role":"user","content":"first"},{"role":"assistant","content":"ok"},{"role":"user","content":"second"}],"stream":false}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	result, err := svc.ForwardAsAnthropic(context.Background(), c, account, body, "stable-cache-key", "gpt-5.4")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, gjson.GetBytes(upstream.lastBody, "previous_response_id").Exists())
}

func TestForwardAsAnthropic_ReusesOAuthCodexTurnState(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	firstResp := openAICompatSSECompletedResponse("resp_oauth_first", "gpt-5.4")
	firstResp.Header.Set("x-codex-turn-state", "turn_state_first")
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		firstResp,
		openAICompatSSECompletedResponse("resp_oauth_second", "gpt-5.4"),
	}}
	svc := &OpenAIGatewayService{
		httpUpstream: upstream,
		cfg:          &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: false}}},
	}
	account := &Account{
		ID:          1,
		Name:        "openai-oauth",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token":       "oauth-token",
			"chatgpt_account_id": "chatgpt-acc",
		},
	}

	firstBody := []byte(`{"model":"claude-sonnet-4-5","max_tokens":16,"messages":[{"role":"user","content":"first"}],"stream":false}`)
	firstRec := httptest.NewRecorder()
	firstCtx, _ := gin.CreateTestContext(firstRec)
	firstCtx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(firstBody))
	firstCtx.Request.Header.Set("Content-Type", "application/json")

	firstResult, err := svc.ForwardAsAnthropic(context.Background(), firstCtx, account, firstBody, "stable-cache-key", "gpt-5.4")
	require.NoError(t, err)
	require.NotNil(t, firstResult)
	require.Empty(t, upstream.requests[0].Header.Get("x-codex-turn-state"))
	require.Empty(t, upstream.requests[0].Header.Get("OpenAI-Beta"))
	require.Empty(t, upstream.requests[0].Header.Get("originator"))

	secondBody := []byte(`{"model":"claude-sonnet-4-5","max_tokens":16,"messages":[{"role":"user","content":"first"},{"role":"assistant","content":"ok"},{"role":"user","content":"second"}],"stream":false}`)
	secondRec := httptest.NewRecorder()
	secondCtx, _ := gin.CreateTestContext(secondRec)
	secondCtx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(secondBody))
	secondCtx.Request.Header.Set("Content-Type", "application/json")

	secondResult, err := svc.ForwardAsAnthropic(context.Background(), secondCtx, account, secondBody, "stable-cache-key", "gpt-5.4")
	require.NoError(t, err)
	require.NotNil(t, secondResult)
	require.Equal(t, "turn_state_first", upstream.requests[1].Header.Get("x-codex-turn-state"))
	require.Equal(t, generateSessionUUID(isolateOpenAISessionID(0, "stable-cache-key")), upstream.requests[1].Header.Get("session_id"))
	require.Empty(t, upstream.requests[1].Header.Get("conversation_id"))
	require.Empty(t, upstream.requests[1].Header.Get("OpenAI-Beta"))
	require.Empty(t, upstream.requests[1].Header.Get("originator"))
	require.False(t, gjson.GetBytes(upstream.bodies[1], "prompt_cache_key").Exists())
	require.False(t, gjson.GetBytes(upstream.bodies[1], "previous_response_id").Exists())
}

func TestForwardAsAnthropic_OAuthDigestFallbackReusesTurnStateWithoutExplicitKey(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	firstResp := openAICompatSSECompletedResponse("resp_oauth_digest_first", "gpt-5.4")
	firstResp.Header.Set("x-codex-turn-state", "turn_state_digest_first")
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		firstResp,
		openAICompatSSECompletedResponse("resp_oauth_digest_second", "gpt-5.4"),
	}}
	svc := &OpenAIGatewayService{
		httpUpstream: upstream,
		cfg:          &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: false}}},
	}
	account := &Account{
		ID:          1,
		Name:        "openai-oauth",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token":       "oauth-token",
			"chatgpt_account_id": "chatgpt-acc",
		},
	}

	firstBody := []byte(`{"model":"claude-sonnet-4-5","max_tokens":16,"messages":[{"role":"user","content":"first"}],"stream":false}`)
	firstRec := httptest.NewRecorder()
	firstCtx, _ := gin.CreateTestContext(firstRec)
	firstCtx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(firstBody))
	firstCtx.Request.Header.Set("Content-Type", "application/json")

	firstResult, err := svc.ForwardAsAnthropic(context.Background(), firstCtx, account, firstBody, "", "gpt-5.4")
	require.NoError(t, err)
	require.NotNil(t, firstResult)
	firstSessionID := upstream.requests[0].Header.Get("session_id")
	require.NotEmpty(t, firstSessionID)
	require.Empty(t, upstream.requests[0].Header.Get("x-codex-turn-state"))
	require.False(t, gjson.GetBytes(upstream.bodies[0], "prompt_cache_key").Exists())

	secondBody := []byte(`{"model":"claude-sonnet-4-5","max_tokens":16,"messages":[{"role":"user","content":"first"},{"role":"assistant","content":"ok"},{"role":"user","content":"second"}],"stream":false}`)
	secondRec := httptest.NewRecorder()
	secondCtx, _ := gin.CreateTestContext(secondRec)
	secondCtx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(secondBody))
	secondCtx.Request.Header.Set("Content-Type", "application/json")

	secondResult, err := svc.ForwardAsAnthropic(context.Background(), secondCtx, account, secondBody, "", "gpt-5.4")
	require.NoError(t, err)
	require.NotNil(t, secondResult)
	require.Equal(t, firstSessionID, upstream.requests[1].Header.Get("session_id"))
	require.Equal(t, "turn_state_digest_first", upstream.requests[1].Header.Get("x-codex-turn-state"))
	require.Empty(t, upstream.requests[1].Header.Get("conversation_id"))
	require.False(t, gjson.GetBytes(upstream.bodies[1], "prompt_cache_key").Exists())
	require.False(t, gjson.GetBytes(upstream.bodies[1], "previous_response_id").Exists())
}

func TestForwardAsAnthropic_OAuthMetadataSessionSurvivesDigestPrefixRewrite(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	firstResp := openAICompatSSECompletedResponse("resp_oauth_metadata_first", "gpt-5.5")
	firstResp.Header.Set("x-codex-turn-state", "turn_state_metadata_first")
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		firstResp,
		openAICompatSSECompletedResponse("resp_oauth_metadata_second", "gpt-5.5"),
	}}
	svc := &OpenAIGatewayService{
		httpUpstream: upstream,
		cfg:          &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: false}}},
	}
	account := &Account{
		ID:          1,
		Name:        "openai-oauth",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token":       "oauth-token",
			"chatgpt_account_id": "chatgpt-acc",
		},
	}
	metadata := `{"user_id":"{\"device_id\":\"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\",\"account_uuid\":\"\",\"session_id\":\"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa\"}"}`

	firstBody := []byte(`{"model":"claude-sonnet-4-5","max_tokens":16,"metadata":` + metadata + `,"messages":[{"role":"user","content":"first plan"}],"stream":false}`)
	firstRec := httptest.NewRecorder()
	firstCtx, _ := gin.CreateTestContext(firstRec)
	firstCtx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(firstBody))
	firstCtx.Request.Header.Set("Content-Type", "application/json")

	firstResult, err := svc.ForwardAsAnthropic(context.Background(), firstCtx, account, firstBody, "", "gpt-5.5")
	require.NoError(t, err)
	require.NotNil(t, firstResult)
	firstSessionID := upstream.requests[0].Header.Get("session_id")
	require.NotEmpty(t, firstSessionID)
	require.Empty(t, upstream.requests[0].Header.Get("x-codex-turn-state"))
	require.False(t, gjson.GetBytes(upstream.bodies[0], "prompt_cache_key").Exists())

	secondBody := []byte(`{"model":"claude-sonnet-4-5","max_tokens":16,"metadata":` + metadata + `,"messages":[{"role":"user","content":"rewritten plan"},{"role":"assistant","content":"ok"},{"role":"user","content":"second"}],"stream":false}`)
	secondRec := httptest.NewRecorder()
	secondCtx, _ := gin.CreateTestContext(secondRec)
	secondCtx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(secondBody))
	secondCtx.Request.Header.Set("Content-Type", "application/json")

	secondResult, err := svc.ForwardAsAnthropic(context.Background(), secondCtx, account, secondBody, "", "gpt-5.5")
	require.NoError(t, err)
	require.NotNil(t, secondResult)
	require.Equal(t, firstSessionID, upstream.requests[1].Header.Get("session_id"))
	require.Equal(t, "turn_state_metadata_first", upstream.requests[1].Header.Get("x-codex-turn-state"))
	require.Empty(t, upstream.requests[1].Header.Get("conversation_id"))
	require.False(t, gjson.GetBytes(upstream.bodies[1], "prompt_cache_key").Exists())
	require.False(t, gjson.GetBytes(upstream.bodies[1], "previous_response_id").Exists())
}

func TestForwardAsAnthropic_OAuthMetadataSessionSurvivesChangingCacheControlAnchor(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	firstResp := openAICompatSSECompletedResponse("resp_oauth_cache_anchor_first", "gpt-5.5")
	firstResp.Header.Set("x-codex-turn-state", "turn_state_cache_anchor_first")
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		firstResp,
		openAICompatSSECompletedResponse("resp_oauth_cache_anchor_second", "gpt-5.5"),
	}}
	svc := &OpenAIGatewayService{
		httpUpstream: upstream,
		cfg:          &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: false}}},
	}
	account := &Account{
		ID:          1,
		Name:        "openai-oauth",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token":       "oauth-token",
			"chatgpt_account_id": "chatgpt-acc",
		},
	}
	metadata := `{"user_id":"{\"device_id\":\"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb\",\"account_uuid\":\"\",\"session_id\":\"bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb\"}"}`

	firstBody := []byte(`{"model":"claude-sonnet-4-5","max_tokens":16,"metadata":` + metadata + `,"system":[{"type":"text","text":"anchor one","cache_control":{"type":"ephemeral"}}],"messages":[{"role":"user","content":"first"}],"stream":false}`)
	firstRec := httptest.NewRecorder()
	firstCtx, _ := gin.CreateTestContext(firstRec)
	firstCtx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(firstBody))
	firstCtx.Request.Header.Set("Content-Type", "application/json")

	firstResult, err := svc.ForwardAsAnthropic(context.Background(), firstCtx, account, firstBody, "", "gpt-5.5")
	require.NoError(t, err)
	require.NotNil(t, firstResult)
	firstSessionID := upstream.requests[0].Header.Get("session_id")
	require.NotEmpty(t, firstSessionID)
	require.Empty(t, upstream.requests[0].Header.Get("x-codex-turn-state"))
	require.False(t, gjson.GetBytes(upstream.bodies[0], "prompt_cache_key").Exists())

	secondBody := []byte(`{"model":"claude-sonnet-4-5","max_tokens":16,"metadata":` + metadata + `,"system":[{"type":"text","text":"anchor two","cache_control":{"type":"ephemeral"}}],"messages":[{"role":"user","content":"first"},{"role":"assistant","content":"ok"},{"role":"user","content":"second"}],"stream":false}`)
	secondRec := httptest.NewRecorder()
	secondCtx, _ := gin.CreateTestContext(secondRec)
	secondCtx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(secondBody))
	secondCtx.Request.Header.Set("Content-Type", "application/json")

	secondResult, err := svc.ForwardAsAnthropic(context.Background(), secondCtx, account, secondBody, "", "gpt-5.5")
	require.NoError(t, err)
	require.NotNil(t, secondResult)
	require.Equal(t, firstSessionID, upstream.requests[1].Header.Get("session_id"))
	require.Equal(t, "turn_state_cache_anchor_first", upstream.requests[1].Header.Get("x-codex-turn-state"))
	require.Empty(t, upstream.requests[1].Header.Get("conversation_id"))
	require.False(t, gjson.GetBytes(upstream.bodies[1], "prompt_cache_key").Exists())
	require.False(t, gjson.GetBytes(upstream.bodies[1], "previous_response_id").Exists())
}

func TestForwardAsAnthropic_OAuthKeepsSystemAsDeveloperInput(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	upstream := &httpUpstreamRecorder{resp: openAICompatSSECompletedResponse("resp_oauth_system", "gpt-5.4")}
	svc := &OpenAIGatewayService{
		httpUpstream: upstream,
		cfg:          &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: false}}},
	}
	account := &Account{
		ID:          1,
		Name:        "openai-oauth",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token":       "oauth-token",
			"chatgpt_account_id": "chatgpt-acc",
		},
	}

	body := []byte(`{"model":"claude-sonnet-4-5","max_tokens":16,"system":[{"type":"text","text":"project instructions","cache_control":{"type":"ephemeral"}}],"messages":[{"role":"user","content":"first"}],"stream":false}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	result, err := svc.ForwardAsAnthropic(context.Background(), c, account, body, "", "gpt-5.4")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "developer", gjson.GetBytes(upstream.lastBody, "input.0.role").String())
	require.Equal(t, "input_text", gjson.GetBytes(upstream.lastBody, "input.0.content.0.type").String())
	require.Equal(t, "project instructions", gjson.GetBytes(upstream.lastBody, "input.0.content.0.text").String())
	instructions := gjson.GetBytes(upstream.lastBody, "instructions")
	require.True(t, instructions.Exists())
	require.Empty(t, instructions.String())
	require.Empty(t, upstream.requests[0].Header.Get("OpenAI-Beta"))
	require.Empty(t, upstream.requests[0].Header.Get("originator"))
}

func TestForwardAsAnthropic_OAuthAddsClaudeCodeTodoGuardForCompatModel(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	upstream := &httpUpstreamRecorder{resp: openAICompatSSECompletedResponse("resp_oauth_todo_guard", "gpt-5.5")}
	svc := &OpenAIGatewayService{
		httpUpstream: upstream,
		cfg:          &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: false}}},
	}
	account := &Account{
		ID:          1,
		Name:        "openai-oauth",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token":       "oauth-token",
			"chatgpt_account_id": "chatgpt-acc",
		},
	}

	body := []byte(`{"model":"claude-sonnet-4-5","max_tokens":16,"system":"project instructions","messages":[{"role":"user","content":"review files"}],"stream":false}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	result, err := svc.ForwardAsAnthropic(context.Background(), c, account, body, "", "gpt-5.5")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "developer", gjson.GetBytes(upstream.lastBody, "input.0.role").String())
	require.Equal(t, "project instructions", gjson.GetBytes(upstream.lastBody, "input.0.content.0.text").String())
	require.Equal(t, "developer", gjson.GetBytes(upstream.lastBody, "input.1.role").String())
	require.Contains(t, gjson.GetBytes(upstream.lastBody, "input.1.content.0.text").String(), "<sub2api-claude-code-todo-guard>")
	require.Equal(t, "user", gjson.GetBytes(upstream.lastBody, "input.2.role").String())
}

func TestForwardAsAnthropic_OAuthPreservesClaudeCodeToolCallID(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	upstream := &httpUpstreamRecorder{resp: openAICompatSSECompletedResponse("resp_oauth_tool", "gpt-5.4")}
	svc := &OpenAIGatewayService{
		httpUpstream: upstream,
		cfg:          &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: false}}},
	}
	account := &Account{
		ID:          1,
		Name:        "openai-oauth",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token":       "oauth-token",
			"chatgpt_account_id": "chatgpt-acc",
		},
	}

	body := []byte(`{"model":"claude-sonnet-4-5","max_tokens":16,"messages":[{"role":"user","content":"list files"},{"role":"assistant","content":[{"type":"tool_use","id":"toolu_123","name":"Bash","input":{"command":"ls"}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_123","content":"ok"}]}],"tools":[{"name":"Bash","description":"run shell","input_schema":{"type":"object","properties":{"command":{"type":"string"}}}}],"stream":false}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	result, err := svc.ForwardAsAnthropic(context.Background(), c, account, body, "stable-cache-key", "gpt-5.4")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "toolu_123", gjson.GetBytes(upstream.lastBody, `input.#(type=="function_call").call_id`).String())
	require.Equal(t, "toolu_123", gjson.GetBytes(upstream.lastBody, `input.#(type=="function_call_output").call_id`).String())
	require.True(t, gjson.GetBytes(upstream.lastBody, "parallel_tool_calls").Bool())
	require.Equal(t, "medium", gjson.GetBytes(upstream.lastBody, "text.verbosity").String())
	require.False(t, gjson.GetBytes(upstream.lastBody, "tools.0.strict").Bool())
}

func TestForwardAsAnthropic_StoresStreamingResponseIDWithoutUsage(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	upstream := &httpUpstreamRecorder{}
	svc := &OpenAIGatewayService{
		httpUpstream: upstream,
		cfg:          &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: false}}},
	}
	account := &Account{
		ID:          1,
		Name:        "openai-apikey",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": "https://api.openai.com/v1",
		},
	}

	firstBody := []byte(`{"model":"claude-sonnet-4-5","max_tokens":16,"messages":[{"role":"user","content":"first"}],"stream":true}`)
	upstream.resp = openAICompatSSEResponseWithoutUsage("resp_stream_first", "gpt-5.3-codex")
	firstRec := httptest.NewRecorder()
	firstCtx, _ := gin.CreateTestContext(firstRec)
	firstCtx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(firstBody))
	firstCtx.Request.Header.Set("Content-Type", "application/json")

	firstResult, err := svc.ForwardAsAnthropic(context.Background(), firstCtx, account, firstBody, "stable-cache-key", "gpt-5.3-codex")
	require.NoError(t, err)
	require.NotNil(t, firstResult)
	require.Equal(t, "resp_stream_first", firstResult.ResponseID)

	secondBody := []byte(`{"model":"claude-sonnet-4-5","max_tokens":16,"messages":[{"role":"user","content":"first"},{"role":"assistant","content":"ok"},{"role":"user","content":"second"}],"stream":false}`)
	upstream.resp = openAICompatSSECompletedResponse("resp_stream_second", "gpt-5.3-codex")
	secondRec := httptest.NewRecorder()
	secondCtx, _ := gin.CreateTestContext(secondRec)
	secondCtx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(secondBody))
	secondCtx.Request.Header.Set("Content-Type", "application/json")

	secondResult, err := svc.ForwardAsAnthropic(context.Background(), secondCtx, account, secondBody, "stable-cache-key", "gpt-5.3-codex")
	require.NoError(t, err)
	require.NotNil(t, secondResult)
	require.Equal(t, "resp_stream_first", gjson.GetBytes(upstream.lastBody, "previous_response_id").String())
}

func openAICompatSSECompletedResponse(responseID, model string) *http.Response {
	body := strings.Join([]string{
		`data: {"type":"response.completed","response":{"id":"` + responseID + `","object":"response","model":"` + model + `","status":"completed","output":[{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"ok"}]}],"usage":{"input_tokens":5,"output_tokens":2,"total_tokens":7}}}`,
		"",
		"data: [DONE]",
		"",
	}, "\n")
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "x-request-id": []string{"rid_continuation"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func openAICompatSSEResponseWithoutUsage(responseID, model string) *http.Response {
	body := strings.Join([]string{
		`data: {"type":"response.completed","response":{"id":"` + responseID + `","object":"response","model":"` + model + `","status":"completed","output":[{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"ok"}]}]}}`,
		"",
		"data: [DONE]",
		"",
	}, "\n")
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "x-request-id": []string{"rid_" + responseID}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestForwardAsAnthropic_ForcedCodexInstructionsTemplatePrependsRenderedInstructions(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	templateDir := t.TempDir()
	templatePath := filepath.Join(templateDir, "codex-instructions.md.tmpl")
	require.NoError(t, os.WriteFile(templatePath, []byte("server-prefix\n\n{{ .ExistingInstructions }}"), 0o644))

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"gpt-5.4","max_tokens":16,"system":"client-system","messages":[{"role":"user","content":"hello"}],"stream":false}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstreamBody := strings.Join([]string{
		`data: {"type":"response.completed","response":{"id":"resp_1","object":"response","model":"gpt-5.4","status":"completed","output":[{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"ok"}]}],"usage":{"input_tokens":5,"output_tokens":2,"total_tokens":7}}}`,
		"",
		"data: [DONE]",
		"",
	}, "\n")
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "x-request-id": []string{"rid_forced"}},
		Body:       io.NopCloser(strings.NewReader(upstreamBody)),
	}}

	svc := &OpenAIGatewayService{
		cfg: &config.Config{Gateway: config.GatewayConfig{
			ForcedCodexInstructionsTemplateFile: templatePath,
			ForcedCodexInstructionsTemplate:     "server-prefix\n\n{{ .ExistingInstructions }}",
		}},
		httpUpstream: upstream,
	}
	account := &Account{
		ID:          1,
		Name:        "openai-oauth",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token":       "oauth-token",
			"chatgpt_account_id": "chatgpt-acc",
		},
	}

	result, err := svc.ForwardAsAnthropic(context.Background(), c, account, body, "", "gpt-5.1")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "server-prefix\n\nclient-system", gjson.GetBytes(upstream.lastBody, "instructions").String())
}

func TestForwardAsAnthropic_ForcedCodexInstructionsTemplateUsesCachedTemplateContent(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"gpt-5.4","max_tokens":16,"system":"client-system","messages":[{"role":"user","content":"hello"}],"stream":false}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstreamBody := strings.Join([]string{
		`data: {"type":"response.completed","response":{"id":"resp_1","object":"response","model":"gpt-5.4","status":"completed","output":[{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"ok"}]}],"usage":{"input_tokens":5,"output_tokens":2,"total_tokens":7}}}`,
		"",
		"data: [DONE]",
		"",
	}, "\n")
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "x-request-id": []string{"rid_forced_cached"}},
		Body:       io.NopCloser(strings.NewReader(upstreamBody)),
	}}

	svc := &OpenAIGatewayService{
		cfg: &config.Config{Gateway: config.GatewayConfig{
			ForcedCodexInstructionsTemplateFile: "/path/that/should/not/be/read.tmpl",
			ForcedCodexInstructionsTemplate:     "cached-prefix\n\n{{ .ExistingInstructions }}",
		}},
		httpUpstream: upstream,
	}
	account := &Account{
		ID:          1,
		Name:        "openai-oauth",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token":       "oauth-token",
			"chatgpt_account_id": "chatgpt-acc",
		},
	}

	result, err := svc.ForwardAsAnthropic(context.Background(), c, account, body, "", "gpt-5.1")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "cached-prefix\n\nclient-system", gjson.GetBytes(upstream.lastBody, "instructions").String())
}

func TestForwardAsAnthropic_ClientDisconnectDrainsUpstreamUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Writer = &openAICompatFailingWriter{ResponseWriter: c.Writer, failAfter: 0}
	body := []byte(`{"model":"gpt-5.4","max_tokens":16,"messages":[{"role":"user","content":"hello"}],"stream":true}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstreamBody := strings.Join([]string{
		`data: {"type":"response.created","response":{"id":"resp_1","model":"gpt-5.4","status":"in_progress","output":[]}}`,
		"",
		`data: {"type":"response.output_text.delta","delta":"ok"}`,
		"",
		`data: {"type":"response.completed","response":{"id":"resp_1","object":"response","model":"gpt-5.4","status":"completed","output":[{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"ok"}]}],"usage":{"input_tokens":9,"output_tokens":4,"total_tokens":13,"input_tokens_details":{"cached_tokens":3}}}}`,
		"",
		"data: [DONE]",
		"",
	}, "\n")
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "x-request-id": []string{"rid_disconnect"}},
		Body:       io.NopCloser(strings.NewReader(upstreamBody)),
	}}

	svc := &OpenAIGatewayService{httpUpstream: upstream}
	account := &Account{
		ID:          1,
		Name:        "openai-oauth",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token":       "oauth-token",
			"chatgpt_account_id": "chatgpt-acc",
		},
	}

	result, err := svc.ForwardAsAnthropic(context.Background(), c, account, body, "", "gpt-5.1")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 9, result.Usage.InputTokens)
	require.Equal(t, 4, result.Usage.OutputTokens)
	require.Equal(t, 3, result.Usage.CacheReadInputTokens)
}

func TestForwardAsAnthropic_TerminalUsageWithoutUpstreamCloseReturns(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Writer = &openAICompatFailingWriter{ResponseWriter: c.Writer, failAfter: 0}
	body := []byte(`{"model":"gpt-5.4","max_tokens":16,"messages":[{"role":"user","content":"hello"}],"stream":true}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstreamBody := []byte(`data: {"type":"response.completed","response":{"id":"resp_1","object":"response","model":"gpt-5.4","status":"completed","output":[{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"ok"}]}],"usage":{"input_tokens":15,"output_tokens":6,"total_tokens":21,"input_tokens_details":{"cached_tokens":5}}}}` + "\n\n")
	upstreamStream := newOpenAICompatBlockingReadCloser(upstreamBody)
	defer func() {
		require.NoError(t, upstreamStream.Close())
	}()
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "x-request-id": []string{"rid_terminal_no_close"}},
		Body:       upstreamStream,
	}}

	svc := &OpenAIGatewayService{httpUpstream: upstream}
	account := &Account{
		ID:          1,
		Name:        "openai-oauth",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token":       "oauth-token",
			"chatgpt_account_id": "chatgpt-acc",
		},
	}

	type forwardResult struct {
		result *OpenAIForwardResult
		err    error
	}
	resultCh := make(chan forwardResult, 1)
	go func() {
		result, err := svc.ForwardAsAnthropic(context.Background(), c, account, body, "", "gpt-5.1")
		resultCh <- forwardResult{result: result, err: err}
	}()

	select {
	case got := <-resultCh:
		require.NoError(t, got.err)
		require.NotNil(t, got.result)
		require.Equal(t, 15, got.result.Usage.InputTokens)
		require.Equal(t, 6, got.result.Usage.OutputTokens)
		require.Equal(t, 5, got.result.Usage.CacheReadInputTokens)
	case <-time.After(time.Second):
		require.Fail(t, "ForwardAsAnthropic should return after terminal usage event even if upstream keeps the connection open")
	}
}

func TestForwardAsAnthropic_BufferedTerminalWithoutUpstreamCloseReturns(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"gpt-5.4","max_tokens":16,"messages":[{"role":"user","content":"hello"}],"stream":false}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstreamBody := []byte(`data: {"type":"response.completed","response":{"id":"resp_1","object":"response","model":"gpt-5.4","status":"completed","output":[{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"ok"}]}],"usage":{"input_tokens":15,"output_tokens":6,"total_tokens":21,"input_tokens_details":{"cached_tokens":5}}}}` + "\n\n")
	upstreamStream := newOpenAICompatBlockingReadCloser(upstreamBody)
	defer func() {
		require.NoError(t, upstreamStream.Close())
	}()
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "x-request-id": []string{"rid_buffered_terminal_no_close"}},
		Body:       upstreamStream,
	}}

	svc := &OpenAIGatewayService{httpUpstream: upstream}
	account := &Account{
		ID:          1,
		Name:        "openai-oauth",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token":       "oauth-token",
			"chatgpt_account_id": "chatgpt-acc",
		},
	}

	type forwardResult struct {
		result *OpenAIForwardResult
		err    error
	}
	resultCh := make(chan forwardResult, 1)
	go func() {
		result, err := svc.ForwardAsAnthropic(context.Background(), c, account, body, "", "gpt-5.1")
		resultCh <- forwardResult{result: result, err: err}
	}()

	select {
	case got := <-resultCh:
		require.NoError(t, got.err)
		require.NotNil(t, got.result)
		require.Equal(t, 15, got.result.Usage.InputTokens)
		require.Equal(t, 6, got.result.Usage.OutputTokens)
		require.Equal(t, 5, got.result.Usage.CacheReadInputTokens)
		require.Contains(t, rec.Body.String(), `"stop_reason":"end_turn"`)
	case <-time.After(time.Second):
		require.Fail(t, "ForwardAsAnthropic buffered response should return after terminal usage event even if upstream keeps the connection open")
	}
}

func TestForwardAsAnthropic_DoneSentinelWithoutTerminalReturnsError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"gpt-5.4","max_tokens":16,"messages":[{"role":"user","content":"hello"}],"stream":true}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstreamBody := "data: [DONE]\n\n"
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "x-request-id": []string{"rid_missing_terminal"}},
		Body:       io.NopCloser(strings.NewReader(upstreamBody)),
	}}

	svc := &OpenAIGatewayService{httpUpstream: upstream}
	account := &Account{
		ID:          1,
		Name:        "openai-oauth",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token":       "oauth-token",
			"chatgpt_account_id": "chatgpt-acc",
		},
	}

	result, err := svc.ForwardAsAnthropic(context.Background(), c, account, body, "", "gpt-5.1")
	require.Error(t, err)
	require.Contains(t, err.Error(), "missing terminal event")
	require.NotNil(t, result)
	require.Zero(t, result.Usage.InputTokens)
	require.Zero(t, result.Usage.OutputTokens)
}

func TestForwardAsAnthropic_UpstreamRequestIgnoresClientCancel(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	reqCtx, cancel := context.WithCancel(context.Background())
	body := []byte(`{"model":"gpt-5.4","max_tokens":16,"messages":[{"role":"user","content":"hello"}],"stream":false}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body)).WithContext(reqCtx)
	c.Request.Header.Set("Content-Type", "application/json")
	cancel()

	upstreamBody := strings.Join([]string{
		`data: {"type":"response.completed","response":{"id":"resp_1","object":"response","model":"gpt-5.4","status":"completed","output":[{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"ok"}]}],"usage":{"input_tokens":5,"output_tokens":2,"total_tokens":7}}}`,
		"",
		"data: [DONE]",
		"",
	}, "\n")
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "x-request-id": []string{"rid_ctx"}},
		Body:       io.NopCloser(strings.NewReader(upstreamBody)),
	}}

	svc := &OpenAIGatewayService{httpUpstream: upstream}
	account := &Account{
		ID:          1,
		Name:        "openai-oauth",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token":       "oauth-token",
			"chatgpt_account_id": "chatgpt-acc",
		},
	}

	result, err := svc.ForwardAsAnthropic(reqCtx, c, account, body, "", "gpt-5.1")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, upstream.lastReq)
	require.NoError(t, upstream.lastReq.Context().Err())
}
