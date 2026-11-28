//go:build unit

package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// 原生 /v1/messages 的两个 SSE 用量解析器必须保留 message_start 已上报的
// output_tokens（与 Antigravity 解析器、兼容路径的 mergeAnthropicUsage 一致），
// 否则流在 message_delta 之前中断时已观测到的输出用量会被记成 0。
func messageStartUsageParsers() map[string]func(string, *ClaudeUsage) {
	svc := newMinimalGatewayService()
	return map[string]func(string, *ClaudeUsage){
		"standard":    svc.parseSSEUsage,
		"passthrough": parseSSEUsagePassthrough,
	}
}

func TestMessageStartOutputTokens_RetainedWithoutMessageDelta(t *testing.T) {
	for name, parse := range messageStartUsageParsers() {
		t.Run(name, func(t *testing.T) {
			usage := &ClaudeUsage{}
			parse(`{"type":"message_start","message":{"usage":{"input_tokens":2,"output_tokens":8,"cache_read_input_tokens":1000}}}`, usage)

			require.Equal(t, 8, usage.OutputTokens)
			require.Equal(t, 2, usage.InputTokens)
			require.Equal(t, 1000, usage.CacheReadInputTokens)
		})
	}
}

// message_delta 的 output_tokens 是累计值：必须覆盖起始值，不能与之相加。
func TestMessageStartOutputTokens_CumulativeDeltaOverridesStart(t *testing.T) {
	for name, parse := range messageStartUsageParsers() {
		t.Run(name, func(t *testing.T) {
			usage := &ClaudeUsage{}
			parse(`{"type":"message_start","message":{"usage":{"input_tokens":2,"output_tokens":8}}}`, usage)
			parse(`{"type":"message_delta","usage":{"output_tokens":20}}`, usage)

			require.Equal(t, 20, usage.OutputTokens)
			require.Equal(t, 2, usage.InputTokens)
		})
	}
}

func TestMessageStartOutputTokens_NonPositiveOrMissingIgnored(t *testing.T) {
	cases := map[string]string{
		"missing":  `{"type":"message_start","message":{"usage":{"input_tokens":2}}}`,
		"zero":     `{"type":"message_start","message":{"usage":{"input_tokens":2,"output_tokens":0}}}`,
		"negative": `{"type":"message_start","message":{"usage":{"input_tokens":2,"output_tokens":-3}}}`,
	}
	for name, parse := range messageStartUsageParsers() {
		for caseName, data := range cases {
			t.Run(name+"/"+caseName, func(t *testing.T) {
				usage := &ClaudeUsage{}
				parse(data, usage)

				require.Equal(t, 0, usage.OutputTokens)
				require.Equal(t, 2, usage.InputTokens)
			})
		}
	}
}

// 接线级回归：上游发出 message_start(output_tokens=8) 后断流，流处理器返回的
// 已收集用量必须带上这 8 个输出 token，供部分用量结算。
func TestHandleStreamingResponse_ReadErrorAfterMessageStartKeepsObservedOutputTokens(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newMinimalGatewayService()

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body: &streamReadCloser{
			payload: []byte("data: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":2,\"output_tokens\":8,\"cache_read_input_tokens\":1000}}}\n\n"),
			err:     io.ErrUnexpectedEOF,
		},
	}

	result, err := svc.handleStreamingResponse(context.Background(), resp, c, &Account{ID: 1}, time.Now(), "model", "model", false)

	require.Error(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.usage)
	require.Equal(t, 8, result.usage.OutputTokens)
	require.Equal(t, 2, result.usage.InputTokens)
	require.Equal(t, 1000, result.usage.CacheReadInputTokens)
}
