//go:build unit

package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// GLM 生成中途被切断且没有发出任何终止信号：网关不能再把半截回答伪装成
// response.completed，必须下发 response.failed 并返回错误。
func TestForwardResponses_ChatFallbackTruncatedStreamFailsResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte("{\"model\":\"glm-5.3-flash\",\"input\":\"write a long answer\",\"stream\":true}")
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstreamBody := strings.Join([]string{
		"data: {\"id\":\"chatcmpl_cut\",\"object\":\"chat.completion.chunk\",\"model\":\"glm-5.3-flash\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"half an ans\"},\"finish_reason\":null}]}",
		"",
	}, "\n")
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "x-request-id": []string{"rid_glm_cut"}},
		Body:       io.NopCloser(strings.NewReader(upstreamBody)),
	}}
	svc := &OpenAIGatewayService{
		cfg:          rawChatCompletionsTestConfig(),
		httpUpstream: upstream,
	}

	result, err := svc.Forward(context.Background(), c, forceChatResponsesFallbackAccount(), body)
	require.Error(t, err)
	require.NotNil(t, result)

	respBody := rec.Body.String()
	require.Contains(t, respBody, "event: response.failed")
	require.Contains(t, respBody, "\"status\":\"failed\"")
	require.NotContains(t, respBody, "event: response.completed")
}

// GLM 在第一个字节之前就中断：响应头尚未提交，应走现有 failover 引擎并在同一
// 账号上有界重试（RetryableOnSameAccount），而不是回一个空的假成功。
func TestForwardResponses_ChatFallbackEmptyStreamFailsOver(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte("{\"model\":\"glm-5.3-flash\",\"input\":\"hello\",\"stream\":true}")
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "x-request-id": []string{"rid_glm_empty"}},
		Body:       io.NopCloser(strings.NewReader("")),
	}}
	svc := &OpenAIGatewayService{
		cfg:          rawChatCompletionsTestConfig(),
		httpUpstream: upstream,
	}

	result, err := svc.Forward(context.Background(), c, forceChatResponsesFallbackAccount(), body)
	require.Nil(t, result)

	var failoverErr *UpstreamFailoverError
	require.True(t, errors.As(err, &failoverErr))
	require.Equal(t, http.StatusBadGateway, failoverErr.StatusCode)
	require.True(t, failoverErr.RetryableOnSameAccount)
	require.NotContains(t, rec.Body.String(), "data: [DONE]")
}

// GLM 显式返回 finish_reason=network_error（推理被中断）：即使带了 [DONE] 和
// usage，也必须按失败收尾，不能伪装成 status=completed。
func TestForwardResponses_ChatFallbackGLMNetworkErrorFinishReasonFailsResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte("{\"model\":\"glm-5.3-flash\",\"input\":\"hello\",\"stream\":true}")
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstreamBody := strings.Join([]string{
		"data: {\"id\":\"chatcmpl_glm_net\",\"object\":\"chat.completion.chunk\",\"model\":\"glm-5.3-flash\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"partial\"},\"finish_reason\":null}]}",
		"",
		"data: {\"id\":\"chatcmpl_glm_net\",\"object\":\"chat.completion.chunk\",\"model\":\"glm-5.3-flash\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"network_error\"}],\"usage\":{\"prompt_tokens\":3,\"completion_tokens\":1,\"total_tokens\":4}}",
		"",
		"data: [DONE]",
		"",
	}, "\n")
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "x-request-id": []string{"rid_glm_net"}},
		Body:       io.NopCloser(strings.NewReader(upstreamBody)),
	}}
	svc := &OpenAIGatewayService{
		cfg:          rawChatCompletionsTestConfig(),
		httpUpstream: upstream,
	}

	result, err := svc.Forward(context.Background(), c, forceChatResponsesFallbackAccount(), body)
	require.Error(t, err)
	require.NotNil(t, result)

	respBody := rec.Body.String()
	require.Contains(t, respBody, "event: response.failed")
	require.NotContains(t, respBody, "event: response.completed")
}
