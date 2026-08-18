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

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOpenAIUsageIsEmpty(t *testing.T) {
	t.Parallel()
	require.True(t, openAIUsageIsEmpty(OpenAIUsage{}))
	require.False(t, openAIUsageIsEmpty(OpenAIUsage{InputTokens: 1}))
	require.False(t, openAIUsageIsEmpty(OpenAIUsage{OutputTokens: 1}))
	require.False(t, openAIUsageIsEmpty(OpenAIUsage{CacheReadInputTokens: 1}))
}

func TestResolveCCStreamUsage_KeepsUpstreamUsage(t *testing.T) {
	t.Parallel()
	upstream := OpenAIUsage{InputTokens: 10, OutputTokens: 4, CacheReadInputTokens: 2}
	got := resolveCCStreamUsage(upstream, []byte(`{"messages":[{"role":"user","content":"hello"}]}`), "ignored", "gpt-5.4", "test", "rid")
	require.Equal(t, upstream, got)
}

func TestResolveCCStreamUsage_EstimatesWhenUpstreamOmitsUsage(t *testing.T) {
	t.Parallel()
	body := []byte(`{"model":"gpt-5.4","messages":[{"role":"user","content":"hello world from the client"}]}`)
	got := resolveCCStreamUsage(OpenAIUsage{}, body, "hello from the assistant reply", "gpt-5.4", "test", "rid")
	require.Greater(t, got.InputTokens, 0)
	require.Greater(t, got.OutputTokens, 0)
}

func TestAppendCCStreamOutputText_CollectsContentReasoningAndTools(t *testing.T) {
	t.Parallel()
	content := "hi"
	reasoning := "plan"
	var b strings.Builder
	appendCCStreamOutputText(&b, &apicompat.ChatCompletionsChunk{
		Choices: []apicompat.ChatChunkChoice{
			{
				Delta: apicompat.ChatDelta{
					Content:          &content,
					ReasoningContent: &reasoning,
					ToolCalls: []apicompat.ChatToolCall{
						{Function: apicompat.ChatFunctionCall{Name: "lookup", Arguments: `{"q":"x"}`}},
					},
				},
			},
		},
	})
	require.Equal(t, `hiplanlookup{"q":"x"}`, b.String())
}

func TestForwardAsAnthropic_ForceChatCompletionsStreamingEstimatesMissingUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"model":"gpt-5.4","max_tokens":32,"messages":[{"role":"user","content":"hello from client"}],"stream":true}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	// Upstream ignores include_usage: stream completes with content but no usage chunk.
	upstreamBody := strings.Join([]string{
		`data: {"id":"chatcmpl_missing","object":"chat.completion.chunk","model":"gpt-5.4","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}`,
		"",
		`data: {"id":"chatcmpl_missing","object":"chat.completion.chunk","model":"gpt-5.4","choices":[{"index":0,"delta":{"content":"hello"},"finish_reason":null}]}`,
		"",
		`data: {"id":"chatcmpl_missing","object":"chat.completion.chunk","model":"gpt-5.4","choices":[{"index":0,"delta":{"content":" world"},"finish_reason":null}]}`,
		"",
		`data: {"id":"chatcmpl_missing","object":"chat.completion.chunk","model":"gpt-5.4","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
		"",
		"data: [DONE]",
		"",
	}, "\n")
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "x-request-id": []string{"rid_msg_missing_usage"}},
		Body:       io.NopCloser(strings.NewReader(upstreamBody)),
	}}
	svc := &OpenAIGatewayService{
		cfg:          rawChatCompletionsTestConfig(),
		httpUpstream: upstream,
	}

	result, err := svc.ForwardAsAnthropic(context.Background(), c, forceChatMessagesFallbackAccount(), body, "", "")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, gjson.GetBytes(upstream.lastBody, "stream_options.include_usage").Bool())
	require.Greater(t, result.Usage.InputTokens, 0, "input tokens should be estimated from request")
	require.Greater(t, result.Usage.OutputTokens, 0, "output tokens should be estimated from stream text")
	require.True(t, result.Stream)

	out := rec.Body.String()
	require.Contains(t, out, `"text":"hello"`)
	require.Contains(t, out, "event: message_delta")
	require.Contains(t, out, "event: message_stop")
	// message_delta should carry estimated usage for clients that bill on stream.
	require.Regexp(t, `"input_tokens":[1-9]`, out)
	require.Regexp(t, `"output_tokens":[1-9]`, out)
}

func TestForwardResponses_ForceChatCompletionsStreamingEstimatesMissingUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"model":"gpt-5.4","input":"hello from client","stream":true}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstreamBody := strings.Join([]string{
		`data: {"id":"chatcmpl_missing_resp","object":"chat.completion.chunk","model":"gpt-5.4","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}`,
		"",
		`data: {"id":"chatcmpl_missing_resp","object":"chat.completion.chunk","model":"gpt-5.4","choices":[{"index":0,"delta":{"content":"reply text"},"finish_reason":null}]}`,
		"",
		`data: {"id":"chatcmpl_missing_resp","object":"chat.completion.chunk","model":"gpt-5.4","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
		"",
		"data: [DONE]",
		"",
	}, "\n")
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "x-request-id": []string{"rid_resp_missing_usage"}},
		Body:       io.NopCloser(strings.NewReader(upstreamBody)),
	}}
	svc := &OpenAIGatewayService{
		cfg:          rawChatCompletionsTestConfig(),
		httpUpstream: upstream,
	}

	result, err := svc.Forward(context.Background(), c, forceChatResponsesFallbackAccount(), body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Greater(t, result.Usage.InputTokens, 0)
	require.Greater(t, result.Usage.OutputTokens, 0)
	require.Contains(t, rec.Body.String(), "event: response.completed")
	require.Regexp(t, `"input_tokens":[1-9]`, rec.Body.String())
	require.Regexp(t, `"output_tokens":[1-9]`, rec.Body.String())
}

func TestForwardAsRawChatCompletions_EstimatesMissingStreamUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"model":"gpt-5.4","messages":[{"role":"user","content":"hello from client"}],"stream":true}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstreamBody := strings.Join([]string{
		`data: {"id":"chatcmpl_raw_missing","object":"chat.completion.chunk","model":"gpt-5.4","choices":[{"index":0,"delta":{"content":"ok there"}}]}`,
		"",
		`data: {"id":"chatcmpl_raw_missing","object":"chat.completion.chunk","model":"gpt-5.4","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
		"",
		"data: [DONE]",
		"",
	}, "\n")
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "x-request-id": []string{"rid_raw_missing_usage"}},
		Body:       io.NopCloser(strings.NewReader(upstreamBody)),
	}}
	svc := &OpenAIGatewayService{
		cfg:          rawChatCompletionsTestConfig(),
		httpUpstream: upstream,
	}
	account := rawChatCompletionsTestAccount()

	result, err := svc.forwardAsRawChatCompletions(context.Background(), c, account, body, "")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Greater(t, result.Usage.InputTokens, 0)
	require.Greater(t, result.Usage.OutputTokens, 0)
	// Raw path still passes through upstream chunks as-is; billing uses estimated usage.
	require.Contains(t, rec.Body.String(), `"content":"ok there"`)
}
