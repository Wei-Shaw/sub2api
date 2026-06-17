//go:build unit

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
)

func TestExtractCCReasoningEffortFromBody(t *testing.T) {
	t.Parallel()

	t.Run("nested reasoning.effort", func(t *testing.T) {
		got := extractCCReasoningEffortFromBody([]byte(`{"reasoning":{"effort":"HIGH"}}`))
		require.NotNil(t, got)
		require.Equal(t, "high", *got)
	})

	t.Run("flat reasoning_effort", func(t *testing.T) {
		got := extractCCReasoningEffortFromBody([]byte(`{"reasoning_effort":"x-high"}`))
		require.NotNil(t, got)
		require.Equal(t, "xhigh", *got)
	})

	t.Run("DeepSeek max", func(t *testing.T) {
		got := extractCCReasoningEffortFromBody([]byte(`{"reasoning_effort":"Max"}`))
		require.NotNil(t, got)
		require.Equal(t, "xhigh", *got)
	})

	t.Run("missing effort", func(t *testing.T) {
		require.Nil(t, extractCCReasoningEffortFromBody([]byte(`{"model":"gpt-5"}`)))
	})
}

func TestHandleCCBufferedFromAnthropic_PreservesMessageStartCacheUsageAndReasoning(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	reasoningEffort := "high"
	resp := &http.Response{
		Header: http.Header{"x-request-id": []string{"rid_cc_buffered"}},
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			`event: message_start`,
			`data: {"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","content":[],"model":"claude-sonnet-4.5","stop_reason":"","usage":{"input_tokens":12,"cache_read_input_tokens":9,"cache_creation_input_tokens":3}}}`,
			``,
			`event: content_block_start`,
			`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":"hello"}}`,
			``,
			`event: message_delta`,
			`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":7}}`,
			``,
		}, "\n"))),
	}

	svc := &GatewayService{}
	result, err := svc.handleCCBufferedFromAnthropic(resp, c, "gpt-5", "claude-sonnet-4.5", &reasoningEffort, time.Now(), false)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 12, result.Usage.InputTokens)
	require.Equal(t, 7, result.Usage.OutputTokens)
	require.Equal(t, 9, result.Usage.CacheReadInputTokens)
	require.Equal(t, 3, result.Usage.CacheCreationInputTokens)
	require.NotNil(t, result.ReasoningEffort)
	require.Equal(t, "high", *result.ReasoningEffort)
}

func TestHandleCCBufferedFromAnthropic_ParsesNestedCacheCreationUsage(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	resp := &http.Response{
		Header: http.Header{"x-request-id": []string{"rid_cc_buffered_nested_usage"}},
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			`event: message_start`,
			`data: {"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","content":[],"model":"claude-opus-4-6","stop_reason":"","usage":{"input_tokens":227,"cached_tokens":192,"cache_creation":{"ephemeral_5m_input_tokens":13,"ephemeral_1h_input_tokens":17}}}}`,
			``,
			`event: content_block_start`,
			`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":"hello"}}`,
			``,
			`event: message_delta`,
			`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":69}}`,
			``,
		}, "\n"))),
	}

	svc := &GatewayService{}
	result, err := svc.handleCCBufferedFromAnthropic(resp, c, "claude-opus-4-6", "claude-opus-4-6", nil, time.Now(), false)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 227, result.Usage.InputTokens)
	require.Equal(t, 69, result.Usage.OutputTokens)
	require.Equal(t, 192, result.Usage.CacheReadInputTokens)
	require.Equal(t, 30, result.Usage.CacheCreationInputTokens)
	require.Equal(t, 13, result.Usage.CacheCreation5mTokens)
	require.Equal(t, 17, result.Usage.CacheCreation1hTokens)
}

func TestHandleCCNonStreamingFromAnthropic_NormalizesOneAPIInclusiveCacheUsage(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	resp := &http.Response{
		Header: http.Header{"x-oneapi-request-id": []string{"oneapi_rid_1"}},
		Body:   io.NopCloser(strings.NewReader(`{"id":"msg_1","type":"message","role":"assistant","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","model":"claude-opus-4-6","usage":{"input_tokens":246,"cache_creation_input_tokens":0,"cache_read_input_tokens":192,"output_tokens":103}}`)),
	}

	svc := &GatewayService{}
	result, err := svc.handleCCNonStreamingFromAnthropic(resp, c, "claude-opus-4-6", "claude-opus-4-6", nil, time.Now(), true)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "oneapi_rid_1", result.RequestID)
	require.Equal(t, 54, result.Usage.InputTokens)
	require.Equal(t, 192, result.Usage.CacheReadInputTokens)
	require.Equal(t, 103, result.Usage.OutputTokens)
	require.Contains(t, rec.Body.String(), `"prompt_tokens":246`)
	require.Contains(t, rec.Body.String(), `"cached_tokens":192`)
}

func TestNormalizeInclusiveClaudeCacheUsage_DoesNotDoubleSubtractAlreadyAdjustedCache(t *testing.T) {
	t.Parallel()

	usage := &ClaudeUsage{
		InputTokens:                 97,
		CacheReadInputTokens:        192,
		cacheTokensRemovedFromInput: 192,
	}

	normalizeInclusiveClaudeCacheUsage(usage, true)

	require.Equal(t, 97, usage.InputTokens)
	require.Equal(t, 192, usage.CacheReadInputTokens)
}

func TestHandleCCStreamingFromAnthropic_OneAPIStreamWithoutFinalUsageCannotInventCache(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	resp := &http.Response{
		Header: http.Header{"x-oneapi-request-id": []string{"oneapi_stream_rid"}},
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			`event: message_start`,
			`data: {"type":"message_start","message":{"id":"msg_2","type":"message","role":"assistant","content":[],"model":"claude-opus-4-6","stop_reason":"","usage":{"input_tokens":29,"cache_creation_input_tokens":0,"cache_read_input_tokens":0,"output_tokens":0}}}`,
			``,
			`event: content_block_start`,
			`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
			``,
			`event: content_block_delta`,
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hello world"}}`,
			``,
			`event: message_stop`,
			`data: {"type":"message_stop"}`,
			``,
		}, "\n"))),
	}

	svc := &GatewayService{}
	result, err := svc.handleCCStreamingFromAnthropic(resp, c, "claude-opus-4-6", "claude-opus-4-6", nil, time.Now(), true, true)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "oneapi_stream_rid", result.RequestID)
	require.Equal(t, 29, result.Usage.InputTokens)
	require.Equal(t, 0, result.Usage.CacheReadInputTokens)
	require.Greater(t, result.Usage.OutputTokens, 0)
}

func TestHandleCCStreamingFromAnthropic_PreservesMessageStartCacheUsageAndReasoning(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	reasoningEffort := "medium"
	resp := &http.Response{
		Header: http.Header{"x-request-id": []string{"rid_cc_stream"}},
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			`event: message_start`,
			`data: {"type":"message_start","message":{"id":"msg_2","type":"message","role":"assistant","content":[],"model":"claude-sonnet-4.5","stop_reason":"","usage":{"input_tokens":20,"cache_read_input_tokens":11,"cache_creation_input_tokens":4}}}`,
			``,
			`event: content_block_start`,
			`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":"hello"}}`,
			``,
			`event: message_delta`,
			`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":8}}`,
			``,
			`event: message_stop`,
			`data: {"type":"message_stop"}`,
			``,
		}, "\n"))),
	}

	svc := &GatewayService{}
	result, err := svc.handleCCStreamingFromAnthropic(resp, c, "gpt-5", "claude-sonnet-4.5", &reasoningEffort, time.Now(), true, false)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 20, result.Usage.InputTokens)
	require.Equal(t, 8, result.Usage.OutputTokens)
	require.Equal(t, 11, result.Usage.CacheReadInputTokens)
	require.Equal(t, 4, result.Usage.CacheCreationInputTokens)
	require.NotNil(t, result.ReasoningEffort)
	require.Equal(t, "medium", *result.ReasoningEffort)
	require.Contains(t, rec.Body.String(), `[DONE]`)
}

func TestHandleCCStreamingFromAnthropic_ClientDisconnectDrainsUsage(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Writer = &openAIChatFailingWriter{ResponseWriter: c.Writer, failAfter: 1}

	resp := &http.Response{
		Header: http.Header{"x-request-id": []string{"rid_cc_stream_disconnect_usage"}},
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			`event: message_start`,
			`data: {"type":"message_start","message":{"id":"msg_2","type":"message","role":"assistant","content":[],"model":"claude-opus-4-6","stop_reason":"","usage":{"input_tokens":227,"cached_tokens":192}}}`,
			``,
			`event: content_block_start`,
			`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
			``,
			`event: content_block_delta`,
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hello"}}`,
			``,
			`event: message_delta`,
			`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":69}}`,
			``,
			`event: message_stop`,
			`data: {"type":"message_stop"}`,
			``,
		}, "\n"))),
	}

	svc := &GatewayService{}
	result, err := svc.handleCCStreamingFromAnthropic(resp, c, "claude-opus-4-6", "claude-opus-4-6", nil, time.Now(), true, false)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 227, result.Usage.InputTokens)
	require.Equal(t, 69, result.Usage.OutputTokens)
	require.Equal(t, 192, result.Usage.CacheReadInputTokens)
}

func TestHandleCCStreamingFromAnthropic_DataOnlyFramesParseUsage(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	resp := &http.Response{
		Header: http.Header{"x-request-id": []string{"rid_cc_stream_data_only_usage"}},
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			`data: {"type":"message_start","message":{"id":"msg_3","type":"message","role":"assistant","content":[],"model":"claude-opus-4-6","stop_reason":"","usage":{"input_tokens":39}}}`,
			``,
			`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
			``,
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"你好"}}`,
			``,
			`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":7}}`,
			``,
			`data: {"type":"message_stop"}`,
			``,
		}, "\n"))),
	}

	svc := &GatewayService{}
	result, err := svc.handleCCStreamingFromAnthropic(resp, c, "claude-opus-4-6", "claude-opus-4-6", nil, time.Now(), true, false)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 39, result.Usage.InputTokens)
	require.Equal(t, 7, result.Usage.OutputTokens)
	require.Contains(t, rec.Body.String(), "你好")
	require.Contains(t, rec.Body.String(), `[DONE]`)
}

func TestHandleCCStreamingFromAnthropic_EstimatesFinalUsageBeforeMessageStop(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	resp := &http.Response{
		Header: http.Header{"x-request-id": []string{"rid_cc_stream_estimated_usage"}},
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			`event: message_start`,
			`data: {"type":"message_start","message":{"id":"msg_est","type":"message","role":"assistant","content":[],"model":"claude-opus-4-6","stop_reason":"","usage":{"input_tokens":17,"output_tokens":0}}}`,
			``,
			`event: content_block_start`,
			`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
			``,
			`event: content_block_delta`,
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hello"}}`,
			``,
			`event: message_stop`,
			`data: {"type":"message_stop"}`,
			``,
		}, "\n"))),
	}

	svc := &GatewayService{}
	result, err := svc.handleCCStreamingFromAnthropic(resp, c, "claude-opus-4-6", "claude-opus-4-6", nil, time.Now(), true, false)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 17, result.Usage.InputTokens)
	require.Equal(t, 2, result.Usage.OutputTokens)
	require.Contains(t, rec.Body.String(), `"completion_tokens":2`)
	require.Contains(t, rec.Body.String(), `"total_tokens":19`)
	require.Contains(t, rec.Body.String(), `[DONE]`)
}

func TestHandleCCStreamingFromAnthropic_OpenAIStyleFinalUsageKeepsAnthropicInput(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	resp := &http.Response{
		Header: http.Header{"x-request-id": []string{"rid_cc_stream_openai_style_usage"}},
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			`data: {"type":"message_start","message":{"id":"msg_compat","type":"message","role":"assistant","content":[],"model":"claude-opus-4-6","stop_reason":"","usage":{"input_tokens":91}}}`,
			``,
			`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
			``,
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hello"}}`,
			``,
			`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"prompt_tokens":289,"completion_tokens":199,"prompt_tokens_details":{"cached_tokens":192}}}`,
			``,
			`data: {"type":"message_stop"}`,
			``,
		}, "\n"))),
	}

	svc := &GatewayService{}
	result, err := svc.handleCCStreamingFromAnthropic(resp, c, "claude-opus-4-6", "claude-opus-4-6", nil, time.Now(), true, false)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 91, result.Usage.InputTokens)
	require.Equal(t, 199, result.Usage.OutputTokens)
	require.Equal(t, 192, result.Usage.CacheReadInputTokens)
	require.Contains(t, rec.Body.String(), "hello")
	require.Contains(t, rec.Body.String(), `[DONE]`)
}

func TestHandleCCStreamingFromAnthropic_GeminiStyleUsageMetadata(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	resp := &http.Response{
		Header: http.Header{"x-request-id": []string{"rid_cc_stream_gemini_usage_metadata"}},
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			`data: {"type":"message_start","message":{"id":"msg_gemini_compat","type":"message","role":"assistant","content":[],"model":"claude-opus-4-6","stop_reason":"","usage":{"input_tokens":653}}}`,
			``,
			`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
			``,
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hello"}}`,
			``,
			`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usageMetadata":{"promptTokenCount":642,"candidatesTokenCount":209,"cachedContentTokenCount":448}}`,
			``,
			`data: {"type":"message_stop"}`,
			``,
		}, "\n"))),
	}

	svc := &GatewayService{}
	result, err := svc.handleCCStreamingFromAnthropic(resp, c, "claude-opus-4-6", "claude-opus-4-6", nil, time.Now(), true, false)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 194, result.Usage.InputTokens)
	require.Equal(t, 209, result.Usage.OutputTokens)
	require.Equal(t, 448, result.Usage.CacheReadInputTokens)
	require.Contains(t, rec.Body.String(), "hello")
	require.Contains(t, rec.Body.String(), `[DONE]`)
}

func TestHandleCCStreamingFromAnthropic_EventNameSuppliesMissingType(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	resp := &http.Response{
		Header: http.Header{"x-request-id": []string{"rid_cc_stream_event_type_usage"}},
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			`event: message_start`,
			`data: {"message":{"id":"msg_4","type":"message","role":"assistant","content":[],"model":"claude-opus-4-6","stop_reason":"","usage":{"input_tokens":11}}}`,
			``,
			`event: content_block_start`,
			`data: {"index":0,"content_block":{"type":"text","text":""}}`,
			``,
			`event: content_block_delta`,
			`data: {"index":0,"delta":{"type":"text_delta","text":"hello"}}`,
			``,
			`event: message_delta`,
			`data: {"delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":5}}`,
			``,
			`event: message_stop`,
			`data: {}`,
			``,
		}, "\n"))),
	}

	svc := &GatewayService{}
	result, err := svc.handleCCStreamingFromAnthropic(resp, c, "claude-opus-4-6", "claude-opus-4-6", nil, time.Now(), true, false)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 11, result.Usage.InputTokens)
	require.Equal(t, 5, result.Usage.OutputTokens)
	require.Contains(t, rec.Body.String(), "hello")
}

func TestHandleCCStreamingFromAnthropic_EstimatesOutputWhenUsageMissing(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	resp := &http.Response{
		Header: http.Header{"x-request-id": []string{"rid_cc_stream_usage_fallback"}},
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			`data: {"type":"message_start","message":{"id":"msg_5","type":"message","role":"assistant","content":[],"model":"claude-opus-4-6","stop_reason":"","usage":{"input_tokens":39}}}`,
			``,
			`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
			``,
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hello world"}}`,
			``,
			`data: {"type":"message_stop"}`,
			``,
		}, "\n"))),
	}

	svc := &GatewayService{}
	result, err := svc.handleCCStreamingFromAnthropic(resp, c, "claude-opus-4-6", "claude-opus-4-6", nil, time.Now(), true, false)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 39, result.Usage.InputTokens)
	require.Greater(t, result.Usage.OutputTokens, 0)
	require.Contains(t, rec.Body.String(), "hello world")
}

func TestHandleCCBufferedFromAnthropic_DataOnlyFramesParseUsage(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	resp := &http.Response{
		Header: http.Header{"x-request-id": []string{"rid_cc_buffered_data_only_usage"}},
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			`data: {"type":"message_start","message":{"id":"msg_6","type":"message","role":"assistant","content":[],"model":"claude-opus-4-6","stop_reason":"","usage":{"input_tokens":21}}}`,
			``,
			`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
			``,
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hi"}}`,
			``,
			`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":3}}`,
			``,
			`data: {"type":"message_stop"}`,
			``,
		}, "\n"))),
	}

	svc := &GatewayService{}
	result, err := svc.handleCCBufferedFromAnthropic(resp, c, "claude-opus-4-6", "claude-opus-4-6", nil, time.Now(), false)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 21, result.Usage.InputTokens)
	require.Equal(t, 3, result.Usage.OutputTokens)
	require.Contains(t, rec.Body.String(), `"completion_tokens":3`)
}
