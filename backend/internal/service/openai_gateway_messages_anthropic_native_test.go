//go:build unit

package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// 国产供应商原生 Anthropic 直通路径（api_protocol=anthropic）的 reasoning_effort 记录。
// 回归背景：该路径此前从不提取 Claude 协议的 output_config.effort，也不做
// thinking-enabled 兜底，导致 kimi/zhipu/deepseek 平台分组的 /v1/messages 请求
// usage_log.reasoning_effort 恒为 NULL。

func nativeAnthropicTestAccount() *Account {
REDACTED
		ID:          702,
		Name:        "kimi-native",
		Platform:    PlatformKimi,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
REDACTED
			"api_key":      "sk-test",
			"api_protocol": APIProtocolAnthropic,
			"api_base_urls": map[string]any{
				APIProtocolAnthropic: "http://anthropic.example",
		REDACTED,
	REDACTED,
REDACTED
REDACTED

func nativeAnthropicBufferedResponse() *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"REDACTEDREDACTED,
		Body: io.NopCloser(strings.NewReader(
			`{"id":"msg_1","type":"message","role":"assistant","model":"k3",` +
				`"content":[{"type":"text","text":"pong"REDACTED],"stop_reason":"end_turn",` +
				`"usage":{"input_tokens":93,"output_tokens":16REDACTEDREDACTED`,
		)),
REDACTED
REDACTED

func nativeAnthropicStreamResponse() *http.Response {
	sse := `event: message_start
data: {"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","model":"k3","content":[],"stop_reason":null,"usage":{"input_tokens":93,"output_tokens":1REDACTEDREDACTEDREDACTED

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""REDACTEDREDACTED

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"pong"REDACTEDREDACTED

event: content_block_stop
data: {"type":"content_block_stop","index":0REDACTED

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"end_turn"REDACTED,"usage":{"output_tokens":16REDACTEDREDACTED

event: message_stop
data: {"type":"message_stop"REDACTED

`
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"REDACTEDREDACTED,
		Body:       io.NopCloser(strings.NewReader(sse)),
REDACTED
REDACTED

func TestNativeAnthropicPassthroughRecordsOutputConfigEffort(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"k3","max_tokens":32,"stream":false,` +
		`"output_config":{"effort":"low"REDACTED,` +
		`"messages":[{"role":"user","content":"hi"REDACTED]REDACTED`)
	upstream := &httpUpstreamRecorder{resp: nativeAnthropicBufferedResponse()REDACTED
	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstreamREDACTED

	result, err := svc.ForwardAsAnthropic(context.Background(),
		adaptiveProtocolTestContext("/v1/messages", body), nativeAnthropicTestAccount(), body, "", "")
REDACTED
	require.NotNil(t, result)
	require.NotNil(t, result.ReasoningEffort)
	require.Equal(t, "low", *result.ReasoningEffort)
REDACTED

func TestNativeAnthropicPassthroughThinkingEnabledFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)
	// 未显式传 effort，但 thinking 已启用：k3 属于 passback-required 白名单，应兜底记为 high。
	body := []byte(`{"model":"k3","max_tokens":32,"stream":false,` +
		`"thinking":{"type":"enabled","budget_tokens":1024REDACTED,` +
		`"messages":[{"role":"user","content":"hi"REDACTED]REDACTED`)
	upstream := &httpUpstreamRecorder{resp: nativeAnthropicBufferedResponse()REDACTED
	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstreamREDACTED

	result, err := svc.ForwardAsAnthropic(context.Background(),
		adaptiveProtocolTestContext("/v1/messages", body), nativeAnthropicTestAccount(), body, "", "")
REDACTED
	require.NotNil(t, result)
	require.NotNil(t, result.ReasoningEffort)
	require.Equal(t, "high", *result.ReasoningEffort)
REDACTED

func TestNativeAnthropicPassthroughStreamRecordsEffort(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"k3","max_tokens":32,"stream":true,` +
		`"output_config":{"effort":"max"REDACTED,` +
		`"messages":[{"role":"user","content":"hi"REDACTED]REDACTED`)
	upstream := &httpUpstreamRecorder{resp: nativeAnthropicStreamResponse()REDACTED
	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstreamREDACTED

	result, err := svc.ForwardAsAnthropic(context.Background(),
		adaptiveProtocolTestContext("/v1/messages", body), nativeAnthropicTestAccount(), body, "", "")
REDACTED
	require.NotNil(t, result)
	require.NotNil(t, result.ReasoningEffort)
	require.Equal(t, "max", *result.ReasoningEffort)
REDACTED

func TestNativeAnthropicPassthroughNoEffortStaysNil(t *testing.T) {
	gin.SetMode(gin.TestMode)
	// 既无 output_config.effort 也未启用 thinking：保持 nil，不做语义注入。
	body := []byte(`{"model":"k3","max_tokens":32,"stream":false,` +
		`"messages":[{"role":"user","content":"hi"REDACTED]REDACTED`)
	upstream := &httpUpstreamRecorder{resp: nativeAnthropicBufferedResponse()REDACTED
	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstreamREDACTED

	result, err := svc.ForwardAsAnthropic(context.Background(),
		adaptiveProtocolTestContext("/v1/messages", body), nativeAnthropicTestAccount(), body, "", "")
REDACTED
	require.NotNil(t, result)
	require.Nil(t, result.ReasoningEffort)
REDACTED
