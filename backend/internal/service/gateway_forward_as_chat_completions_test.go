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
		got := extractCCReasoningEffortFromBody([]byte(`{"reasoning":{"effort":"HIGH"REDACTEDREDACTED`))
		require.NotNil(t, got)
		require.Equal(t, "high", *got)
REDACTED)

	t.Run("flat reasoning_effort", func(t *testing.T) {
		got := extractCCReasoningEffortFromBody([]byte(`{"reasoning_effort":"x-high"REDACTED`))
		require.NotNil(t, got)
		require.Equal(t, "xhigh", *got)
REDACTED)

	t.Run("DeepSeek max", func(t *testing.T) {
		got := extractCCReasoningEffortFromBody([]byte(`{"reasoning_effort":"Max"REDACTED`))
		require.NotNil(t, got)
		require.Equal(t, "xhigh", *got)
REDACTED)

	t.Run("missing effort", func(t *testing.T) {
		require.Nil(t, extractCCReasoningEffortFromBody([]byte(`{"model":"gpt-5"REDACTED`)))
REDACTED)
REDACTED

func TestHandleCCBufferedFromAnthropic_PreservesMessageStartCacheUsageAndReasoning(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	reasoningEffort := "high"
	resp := &http.Response{
		Header: http.Header{"x-request-id": []string{"rid_cc_buffered"REDACTEDREDACTED,
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			`event: message_start`,
			`data: {"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","content":[],"model":"claude-sonnet-4.5","stop_reason":"","usage":{"input_tokens":12,"cache_read_input_tokens":9,"cache_creation_input_tokens":3REDACTEDREDACTEDREDACTED`,
			``,
			`event: content_block_start`,
			`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":"hello"REDACTEDREDACTED`,
			``,
			`event: message_delta`,
			`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"REDACTED,"usage":{"output_tokens":7REDACTEDREDACTED`,
			``,
	REDACTED, "\n"))),
REDACTED

	svc := &GatewayService{REDACTED
	result, err := svc.handleCCBufferedFromAnthropic(resp, c, "gpt-5", "claude-sonnet-4.5", &reasoningEffort, time.Now())
REDACTED
	require.NotNil(t, result)
	require.Equal(t, 12, result.Usage.InputTokens)
	require.Equal(t, 7, result.Usage.OutputTokens)
	require.Equal(t, 9, result.Usage.CacheReadInputTokens)
	require.Equal(t, 3, result.Usage.CacheCreationInputTokens)
	require.NotNil(t, result.ReasoningEffort)
	require.Equal(t, "high", *result.ReasoningEffort)
REDACTED

// Kimi 等 Anthropic 兼容上游返回 SSE 紧凑格式（冒号后无空格），CC 桥此前按
// "event: " / "data: " 严格匹配会丢弃全部事件，最终报 "Upstream stream ended
// without a response"（#4653 同根因；#4657 只修了 /v1/responses 桥）。
func TestHandleCCBufferedFromAnthropic_CompactSSEFormat(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	resp := &http.Response{
		Header: http.Header{"x-request-id": []string{"rid_cc_buffered_compact"REDACTEDREDACTED,
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			`event:message_start`,
			`data:{"type":"message_start","message":{"id":"msg_c1","type":"message","role":"assistant","content":[],"model":"k3","stop_reason":"","usage":{"input_tokens":15,"cache_read_input_tokens":5,"cache_creation_input_tokens":2REDACTEDREDACTEDREDACTED`,
			``,
			`event:content_block_start`,
			`data:{"type":"content_block_start","index":0,"content_block":{"type":"text","text":"OK"REDACTEDREDACTED`,
			``,
			`event:message_delta`,
			`data:{"type":"message_delta","delta":{"stop_reason":"end_turn"REDACTED,"usage":{"output_tokens":3REDACTEDREDACTED`,
			``,
	REDACTED, "\n"))),
REDACTED

	svc := &GatewayService{REDACTED
	result, err := svc.handleCCBufferedFromAnthropic(resp, c, "k3", "k3", nil, time.Now())
REDACTED
	require.NotNil(t, result)
	require.Equal(t, 15, result.Usage.InputTokens)
	require.Equal(t, 3, result.Usage.OutputTokens)
	require.Equal(t, 5, result.Usage.CacheReadInputTokens)
	require.Equal(t, 2, result.Usage.CacheCreationInputTokens)
	require.Contains(t, rec.Body.String(), `"OK"`, "紧凑格式事件必须被解析并产出响应内容")
REDACTED

func TestHandleCCStreamingFromAnthropic_CompactSSEFormat(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	resp := &http.Response{
		Header: http.Header{"x-request-id": []string{"rid_cc_stream_compact"REDACTEDREDACTED,
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			`event:message_start`,
			`data:{"type":"message_start","message":{"id":"msg_c2","type":"message","role":"assistant","content":[],"model":"k3","stop_reason":"","usage":{"input_tokens":21,"cache_read_input_tokens":6,"cache_creation_input_tokens":1REDACTEDREDACTEDREDACTED`,
			``,
			`event:content_block_start`,
			`data:{"type":"content_block_start","index":0,"content_block":{"type":"text","text":"OK"REDACTEDREDACTED`,
			``,
			`event:message_delta`,
			`data:{"type":"message_delta","delta":{"stop_reason":"end_turn"REDACTED,"usage":{"output_tokens":4REDACTEDREDACTED`,
			``,
			`event:message_stop`,
			`data:{"type":"message_stop"REDACTED`,
			``,
	REDACTED, "\n"))),
REDACTED

	svc := &GatewayService{REDACTED
	result, err := svc.handleCCStreamingFromAnthropic(resp, c, "k3", "k3", nil, time.Now(), true)
REDACTED
	require.NotNil(t, result)
	require.Equal(t, 21, result.Usage.InputTokens)
	require.Equal(t, 4, result.Usage.OutputTokens)
	require.Equal(t, 6, result.Usage.CacheReadInputTokens)
	require.Equal(t, 1, result.Usage.CacheCreationInputTokens)
	require.Contains(t, rec.Body.String(), `[DONE]`)
REDACTED

func TestHandleCCStreamingFromAnthropic_PreservesMessageStartCacheUsageAndReasoning(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	reasoningEffort := "medium"
	resp := &http.Response{
		Header: http.Header{"x-request-id": []string{"rid_cc_stream"REDACTEDREDACTED,
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			`event: message_start`,
			`data: {"type":"message_start","message":{"id":"msg_2","type":"message","role":"assistant","content":[],"model":"claude-sonnet-4.5","stop_reason":"","usage":{"input_tokens":20,"cache_read_input_tokens":11,"cache_creation_input_tokens":4REDACTEDREDACTEDREDACTED`,
			``,
			`event: content_block_start`,
			`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":"hello"REDACTEDREDACTED`,
			``,
			`event: message_delta`,
			`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"REDACTED,"usage":{"output_tokens":8REDACTEDREDACTED`,
			``,
			`event: message_stop`,
			`data: {"type":"message_stop"REDACTED`,
			``,
	REDACTED, "\n"))),
REDACTED

	svc := &GatewayService{REDACTED
	result, err := svc.handleCCStreamingFromAnthropic(resp, c, "gpt-5", "claude-sonnet-4.5", &reasoningEffort, time.Now(), true)
REDACTED
	require.NotNil(t, result)
	require.Equal(t, 20, result.Usage.InputTokens)
	require.Equal(t, 8, result.Usage.OutputTokens)
	require.Equal(t, 11, result.Usage.CacheReadInputTokens)
	require.Equal(t, 4, result.Usage.CacheCreationInputTokens)
	require.NotNil(t, result.ReasoningEffort)
	require.Equal(t, "medium", *result.ReasoningEffort)
	require.Contains(t, rec.Body.String(), `[DONE]`)
REDACTED
